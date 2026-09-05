package hub

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	DialTimeout      = 15 * time.Second
	HandshakeTimeout = 20 * time.Second
	IdleTimeout      = 45 * time.Second
	KeepAlive        = 10 * time.Second
)

type Conn interface {
	ReadMsg() (*Message, error)
	WriteMsg(Message) error
	Close() error
	RemoteAddr() net.Addr
	Protocol() string
	RTT() time.Duration
	Dump() []byte
}

type session struct {
	c          Conn
	role       Role
	ed         []byte
	x          []byte
	name       string
	cloud      bool
	mu         sync.Mutex
	rtt        time.Duration
	last       time.Time
	listenV6   string
	listenV4   string
	listenPort uint16
}

func (s *session) send(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.c.WriteMsg(msg)
}

type frameConn struct {
	rw      io.ReadWriteCloser
	remote  net.Addr
	proto   string
	rttFn   func() time.Duration
	mu      sync.Mutex
	dump    []byte
	capture bool
}

func (f *frameConn) ReadMsg() (*Message, error) {
	r := io.Reader(f.rw)
	if f.capture {
		r = io.TeeReader(f.rw, &capturing{f: f})
	}
	return ReadFrame(r)
}

func (f *frameConn) WriteMsg(msg Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capture {
		body, _ := json.Marshal(msg)
		f.dump = append(f.dump, body...)
	}
	return WriteFrame(f.rw, msg)
}

func (f *frameConn) Close() error         { return f.rw.Close() }
func (f *frameConn) RemoteAddr() net.Addr { return f.remote }
func (f *frameConn) Protocol() string     { return f.proto }
func (f *frameConn) RTT() time.Duration {
	if f.rttFn != nil {
		return f.rttFn()
	}
	return 0
}
func (f *frameConn) Dump() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.dump...)
}

type capturing struct{ f *frameConn }

func (c *capturing) Write(p []byte) (int, error) {
	c.f.mu.Lock()
	c.f.dump = append(c.f.dump, p...)
	c.f.mu.Unlock()
	return len(p), nil
}

func quicConfig() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:       IdleTimeout,
		KeepAlivePeriod:      KeepAlive,
		HandshakeIdleTimeout: HandshakeTimeout,
	}
}

type quicStreamCloser struct {
	s quic.Stream
	c quic.Connection
}

func (q quicStreamCloser) Read(p []byte) (int, error)  { return q.s.Read(p) }
func (q quicStreamCloser) Write(p []byte) (int, error) { return q.s.Write(p) }
func (q quicStreamCloser) Close() error {
	_ = q.s.Close()
	return q.c.CloseWithError(0, "bye")
}

func Dial(ctx context.Context, network, addr string, capture bool) (Conn, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DialTimeout)
		defer cancel()
	}
	switch network {
	case "quic", "udp", "udp6", "udp4":
		c, err := quic.DialAddr(ctx, addr, clientTLS(), quicConfig())
		if err != nil {
			return nil, err
		}
		st, err := c.OpenStreamSync(ctx)
		if err != nil {
			_ = c.CloseWithError(1, "stream")
			return nil, err
		}
		return &frameConn{
			rw:      quicStreamCloser{s: st, c: c},
			remote:  c.RemoteAddr(),
			proto:   "quic",
			capture: capture,
		}, nil
	case "tls", "tcp", "tcp6", "tcp4":
		netw := "tcp"
		if network == "tcp6" {
			netw = "tcp6"
		}
		if network == "tcp4" {
			netw = "tcp4"
		}
		d := tls.Dialer{Config: clientTLS(), NetDialer: &net.Dialer{Timeout: DialTimeout}}
		nc, err := d.DialContext(ctx, netw, addr)
		if err != nil {
			return nil, err
		}
		return &frameConn{rw: nc, remote: nc.RemoteAddr(), proto: "tls", capture: capture}, nil
	default:
		return nil, fmt.Errorf("unknown network %s", network)
	}
}

// DualListener accepts QUIC (UDP) and TCP/TLS on the same port.
type DualListener struct {
	conns chan Conn
	stop  context.CancelFunc
	wg    sync.WaitGroup
	Addr  string
	ql    *quic.Listener
	tcp   net.Listener
}

func StartDual(ctx context.Context, tlsConf *tls.Config, udp net.PacketConn, tcp net.Listener, capture bool) (*DualListener, error) {
	ctx, cancel := context.WithCancel(ctx)
	ql, err := quic.Listen(udp, tlsConf, quicConfig())
	if err != nil {
		cancel()
		return nil, err
	}
	d := &DualListener{
		conns: make(chan Conn, 16),
		stop:  cancel,
		Addr:  udp.LocalAddr().String(),
		ql:    ql,
		tcp:   tcp,
	}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			c, err := ql.Accept(ctx)
			if err != nil {
				return
			}
			d.wg.Add(1)
			go func(c quic.Connection) {
				defer d.wg.Done()
				st, err := c.AcceptStream(ctx)
				if err != nil {
					_ = c.CloseWithError(1, "stream")
					return
				}
				select {
				case d.conns <- &frameConn{rw: quicStreamCloser{s: st, c: c}, remote: c.RemoteAddr(), proto: "quic", capture: capture}:
				case <-ctx.Done():
					_ = c.CloseWithError(0, "stop")
				}
			}(c)
		}
	}()
	if tcp != nil {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			for {
				nc, err := tcp.Accept()
				if err != nil {
					return
				}
				tc := tls.Server(nc, tlsConf)
				select {
				case d.conns <- &frameConn{rw: tc, remote: nc.RemoteAddr(), proto: "tls", capture: capture}:
				case <-ctx.Done():
					_ = tc.Close()
					return
				}
			}
		}()
	}
	go func() {
		<-ctx.Done()
		_ = ql.Close()
		if tcp != nil {
			_ = tcp.Close()
		}
	}()
	return d, nil
}

func (d *DualListener) Accept(ctx context.Context) (Conn, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case c, ok := <-d.conns:
		if !ok || c == nil {
			return nil, errors.New("listener closed")
		}
		return c, nil
	}
}

func (d *DualListener) Close() {
	d.stop()
}

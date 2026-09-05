package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mhalaba/Starmesh/invite"
)

type ServerConfig struct {
	Name      string
	Host      string // empty = all interfaces; "::1" for lab
	Port      int
	IPv6Only  bool
	ClaimIPv4 bool
	PublicV6  net.IP
	PublicV4  net.IP
	CloudSeed bool
	Dev       bool
	Capture   bool
	Peers     []string // invite blobs of other hubs
	Logger    *slog.Logger
}

type Server struct {
	cfg    ServerConfig
	id     *Identity
	log    *slog.Logger
	dual   *DualListener
	seen   *seenIDs
	mu     sync.RWMutex
	spokes map[string]*session
	hubs   map[string]*session
	where  map[string]string // spoke ed hex -> hub ed hex
	names  map[string]string
	xpubs  map[string][]byte
	gseen  map[string]uint64
	seq    atomic.Uint64
	inv    *invite.Invite
	cancel context.CancelFunc
	dumps  [][]byte
}

func NewServer(id *Identity, cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Name == "" {
		cfg.Name = id.Name
	}
	return &Server{
		cfg:    cfg,
		id:     id,
		log:    cfg.Logger,
		seen:   newSeen(),
		spokes: map[string]*session{},
		hubs:   map[string]*session{},
		where:  map[string]string{},
		names:  map[string]string{},
		xpubs:  map[string][]byte{},
		gseen:  map[string]uint64{},
	}
}

func (s *Server) Invite() *invite.Invite { return s.inv }

func (s *Server) Start(ctx context.Context) error {
	tlsConf, err := selfSignedTLS(s.id)
	if err != nil {
		return err
	}
	udp, err := listenUDP(s.cfg.Host, s.cfg.Port, s.cfg.IPv6Only || s.cfg.PublicV4 == nil)
	if err != nil {
		return fmt.Errorf("udp listen: %w", err)
	}
	if _, p, err := net.SplitHostPort(udp.LocalAddr().String()); err == nil {
		var port int
		if _, err := fmt.Sscanf(p, "%d", &port); err == nil {
			s.cfg.Port = port
		}
	}
	tcp, err := listenTCP(s.cfg.Host, s.cfg.Port, s.cfg.IPv6Only || s.cfg.PublicV4 == nil)
	if err != nil {
		_ = udp.Close()
		return fmt.Errorf("tcp listen: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	dual, err := StartDual(ctx, tlsConf, udp, tcp, s.cfg.Capture)
	if err != nil {
		cancel()
		_ = udp.Close()
		_ = tcp.Close()
		return err
	}
	s.dual = dual
	if _, port, err := net.SplitHostPort(dual.Addr); err == nil {
		var p int
		if _, err := fmt.Sscanf(port, "%d", &p); err == nil {
			s.cfg.Port = p
		}
	}
	s.inv = s.buildInvite()
	s.log.Info("hub listening",
		"name", s.cfg.Name,
		"addr", dual.Addr,
		"ipv6_only", s.cfg.IPv6Only || !s.cfg.ClaimIPv4,
		"cloud_seed", s.cfg.CloudSeed,
		"short", s.inv.ShortDisplay(),
	)
	go s.acceptLoop(ctx)
	go s.keepAlive(ctx)
	for _, p := range s.cfg.Peers {
		inv, err := invite.DecodeFull(p)
		if err != nil {
			s.log.Warn("peer invite", "err", err)
			continue
		}
		go s.dialPeer(ctx, inv)
	}
	return nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.dual != nil {
		s.dual.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sp := range s.spokes {
		_ = sp.c.Close()
	}
	for _, h := range s.hubs {
		_ = h.c.Close()
	}
}

func (s *Server) buildInvite() *invite.Invite {
	inv := &invite.Invite{
		Version:   invite.Version,
		Port:      uint16(s.cfg.Port),
		Expiry:    time.Now().Add(7 * 24 * time.Hour).UTC(),
		Name:      s.cfg.Name,
		CloudSeed: s.cfg.CloudSeed,
	}
	copy(inv.PubKey[:], s.id.EdPub)
	if s.cfg.PublicV6 != nil {
		inv.IPv6 = s.cfg.PublicV6
	} else if s.cfg.Dev {
		inv.IPv6 = net.ParseIP("::1")
	}
	if s.cfg.ClaimIPv4 && s.cfg.PublicV4 != nil {
		inv.IPv4 = s.cfg.PublicV4
	}
	return inv
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		c, err := s.dual.Accept(ctx)
		if err != nil {
			return
		}
		go s.handleConn(ctx, c)
	}
}

func (s *Server) handleConn(ctx context.Context, c Conn) {
	defer c.Close()
	_ = setReadDeadline(c, HandshakeTimeout)
	msg, err := c.ReadMsg()
	if err != nil {
		return
	}
	if msg.Type != TypeHello && msg.Type != TypePeerHello {
		return
	}
	var h Hello
	if err := json.Unmarshal(msg.Body, &h); err != nil || h.Verify() != nil {
		return
	}
	ok := HelloOK{
		Ed25519:    s.id.EdPub,
		X25519:     s.id.XPub[:],
		Name:       s.cfg.Name,
		ObservedIP: hostOnly(c.RemoteAddr()),
		Ts:         nowUnix(),
		CloudSeed:  s.cfg.CloudSeed,
	}
	ok.Sign(s.id.EdPriv)
	replyType := TypeHelloOK
	if msg.Type == TypePeerHello || h.Role == RoleHub {
		replyType = TypePeerOK
	}
	if err := c.WriteMsg(msgOf(replyType, ok)); err != nil {
		return
	}

	sess := &session{
		c: c, role: h.Role, ed: h.Ed25519, x: h.X25519, name: h.Name,
		cloud: h.CloudSeed, last: time.Now(),
		listenV6: h.ListenIPv6, listenV4: h.ListenIPv4, listenPort: h.ListenPort,
	}
	key := mustHex(h.Ed25519)
	s.mu.Lock()
	if h.Role == RoleHub || msg.Type == TypePeerHello {
		s.hubs[key] = sess
	} else {
		s.spokes[key] = sess
		s.where[key] = mustHex(s.id.EdPub)
		s.names[key] = h.Name
	}
	s.mu.Unlock()
	defer s.drop(key, h.Role)

	s.log.Info("connected", "role", h.Role, "name", h.Name, "fp", Fingerprint(h.Ed25519), "via", c.Protocol())
	s.gossipLocal()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = setReadDeadline(c, IdleTimeout)
		m, err := c.ReadMsg()
		if err != nil {
			return
		}
		if s.cfg.Capture {
			s.mu.Lock()
			s.dumps = append(s.dumps, c.Dump())
			s.mu.Unlock()
		}
		s.onMsg(sess, m)
	}
}

func (s *Server) onMsg(sess *session, m *Message) {
	sess.last = time.Now()
	switch m.Type {
	case TypePing:
		var p Ping
		_ = json.Unmarshal(m.Body, &p)
		_ = sess.send(msgOf(TypePong, Ping{T: p.T}))
	case TypePong:
		var p Ping
		_ = json.Unmarshal(m.Body, &p)
		if p.T > 0 {
			sess.rtt = time.Since(time.Unix(0, p.T))
		}
	case TypeEnvelope:
		var env Envelope
		if err := json.Unmarshal(m.Body, &env); err != nil {
			return
		}
		s.forward(&env, sess)
	case TypePresence:
		var p Presence
		if err := json.Unmarshal(m.Body, &p); err != nil {
			return
		}
		s.mu.Lock()
		s.names[mustHex(sess.ed)] = p.Name
		if len(p.X25519) == 32 {
			sess.x = append([]byte(nil), p.X25519...)
		}
		for _, pk := range p.PubKeys {
			if sess.role == RoleSpoke {
				s.where[mustHex(pk)] = mustHex(s.id.EdPub)
			} else {
				s.where[mustHex(pk)] = mustHex(sess.ed)
			}
		}
		s.mu.Unlock()
		s.gossipLocal()
	case TypePresenceGossip:
		var g PresenceGossip
		if err := json.Unmarshal(m.Body, &g); err != nil {
			return
		}
		origin := mustHex(g.Hub)
		s.mu.Lock()
		if prev, ok := s.gseen[origin]; ok && g.Seq <= prev {
			s.mu.Unlock()
			return
		}
		s.gseen[origin] = g.Seq
		for _, e := range g.Entries {
			s.where[mustHex(e.Pub)] = origin
			if e.Name != "" {
				s.names[mustHex(e.Pub)] = e.Name
			}
			if len(e.X25519) == 32 {
				s.xpubs[mustHex(e.Pub)] = append([]byte(nil), e.X25519...)
			}
		}
		s.mu.Unlock()
		s.fanoutGossip(g, sess)
	case TypePeerHello, TypeHello:
		// already handled
	}
}

func (s *Server) forward(env *Envelope, from *session) {
	if err := env.Verify(); err != nil {
		s.log.Debug("drop envelope", "err", err)
		return
	}
	if !s.seen.Add(env.ID) {
		return
	}
	to := mustHex(env.To)
	s.mu.RLock()
	if sp, ok := s.spokes[to]; ok {
		s.mu.RUnlock()
		_ = sp.send(msgOf(TypeEnvelope, env))
		return
	}
	hubHex, ok := s.where[to]
	var peer *session
	if ok {
		peer = s.hubs[hubHex]
	}
	hubs := make([]*session, 0, len(s.hubs))
	for _, h := range s.hubs {
		if h != from {
			hubs = append(hubs, h)
		}
	}
	s.mu.RUnlock()
	if peer != nil && peer != from {
		_ = peer.send(msgOf(TypeEnvelope, env))
		return
	}
	for _, h := range hubs {
		_ = h.send(msgOf(TypeEnvelope, env))
	}
}

func (s *Server) gossipLocal() {
	g := PresenceGossip{Hub: s.id.EdPub, Seq: s.seq.Add(1)}
	s.mu.Lock()
	s.gseen[mustHex(s.id.EdPub)] = g.Seq
	bl := newBloom()
	self := mustHex(s.id.EdPub)
	for k, sp := range s.spokes {
		pk, _ := parseHex(k)
		g.Entries = append(g.Entries, GossipEntry{Pub: pk, X25519: sp.x, Name: sp.name})
		bl.Add(pk)
		s.xpubs[k] = append([]byte(nil), sp.x...)
		s.where[k] = self
	}
	for k, hubHex := range s.where {
		if hubHex == self {
			continue
		}
		pk, err := parseHex(k)
		if err != nil {
			continue
		}
		g.Entries = append(g.Entries, GossipEntry{Pub: pk, X25519: s.xpubs[k], Name: s.names[k]})
	}
	targets := make([]*session, 0, len(s.hubs)+len(s.spokes))
	for _, h := range s.hubs {
		targets = append(targets, h)
	}
	for _, sp := range s.spokes {
		targets = append(targets, sp)
	}
	s.mu.Unlock()
	if len(g.Entries) >= 64 {
		g.Bloom = bl.b
	}
	msg := msgOf(TypePresenceGossip, g)
	for _, h := range targets {
		_ = h.send(msg)
	}
}

func (s *Server) fanoutGossip(g PresenceGossip, from *session) {
	msg := msgOf(TypePresenceGossip, g)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sp := range s.spokes {
		_ = sp.send(msg)
	}
	for _, h := range s.hubs {
		if h == from {
			continue
		}
		_ = h.send(msg)
	}
}

func (s *Server) drop(key string, role Role) {
	s.mu.Lock()
	if role == RoleHub {
		delete(s.hubs, key)
	} else {
		delete(s.spokes, key)
	}
	s.mu.Unlock()
	s.gossipLocal()
}

func (s *Server) keepAlive(ctx context.Context) {
	t := time.NewTicker(KeepAlive)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ping := msgOf(TypePing, Ping{T: time.Now().UnixNano()})
			s.mu.RLock()
			all := append(values(s.spokes), values(s.hubs)...)
			s.mu.RUnlock()
			for _, sess := range all {
				_ = sess.send(ping)
			}
		}
	}
}

func (s *Server) dialPeer(ctx context.Context, inv *invite.Invite) {
	addrs := []string{}
	if inv.IPv6 != nil {
		addrs = append(addrs, inv.AddrIPv6())
	}
	if inv.IPv4 != nil {
		addrs = append(addrs, inv.AddrIPv4())
	}
	var last error
	for _, addr := range addrs {
		for _, netw := range []string{"quic", "tls"} {
			c, err := Dial(ctx, netw, addr, s.cfg.Capture)
			if err != nil {
				last = err
				continue
			}
			h := Hello{
				Role:       RoleHub,
				Ed25519:    s.id.EdPub,
				X25519:     s.id.XPub[:],
				Name:       s.cfg.Name,
				Ts:         nowUnix(),
				CloudSeed:  s.cfg.CloudSeed,
				ListenIPv6: ipString(s.cfg.PublicV6),
				ListenIPv4: ipString(s.cfg.PublicV4),
				ListenPort: uint16(s.cfg.Port),
			}
			h.Sign(s.id.EdPriv)
			if err := c.WriteMsg(msgOf(TypePeerHello, h)); err != nil {
				_ = c.Close()
				last = err
				continue
			}
			_ = setReadDeadline(c, HandshakeTimeout)
			m, err := c.ReadMsg()
			if err != nil {
				_ = c.Close()
				last = err
				continue
			}
			var ok HelloOK
			if err := json.Unmarshal(m.Body, &ok); err != nil || ok.Verify() != nil {
				_ = c.Close()
				last = errBadHello
				continue
			}
			if len(inv.PubKey) == 32 && !edEq(ok.Ed25519, inv.PubKey[:]) {
				_ = c.Close()
				s.log.Warn("peer key mismatch")
				continue
			}
			go s.handlePeerSession(ctx, c, ok)
			return
		}
	}
	if last != nil {
		s.log.Warn("peer dial failed", "err", last, "name", inv.Name)
	}
}

func (s *Server) handlePeerSession(ctx context.Context, c Conn, ok HelloOK) {
	defer c.Close()
	sess := &session{c: c, role: RoleHub, ed: ok.Ed25519, x: ok.X25519, name: ok.Name, last: time.Now()}
	key := mustHex(ok.Ed25519)
	s.mu.Lock()
	s.hubs[key] = sess
	s.mu.Unlock()
	defer s.drop(key, RoleHub)
	s.log.Info("hub peer", "name", ok.Name, "fp", Fingerprint(ok.Ed25519))
	s.gossipLocal()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = setReadDeadline(c, IdleTimeout)
		m, err := c.ReadMsg()
		if err != nil {
			return
		}
		s.onMsg(sess, m)
	}
}

func (s *Server) Status() []HubStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []HubStatus
	out = append(out, HubStatus{
		Name: s.cfg.Name, Role: "hub", Self: true,
		Fingerprint: s.id.Fingerprint(),
		IPv6:        ipString(s.cfg.PublicV6),
		IPv4:        ipString(s.cfg.PublicV4),
		CloudSeed:   s.cfg.CloudSeed,
		Spokes:      len(s.spokes),
		Peers:       len(s.hubs),
	})
	for _, sp := range s.spokes {
		out = append(out, HubStatus{
			Name: sp.name, Role: "spoke", Fingerprint: Fingerprint(sp.ed),
			RTTMs: sp.rtt.Milliseconds(), Proto: sp.c.Protocol(),
		})
	}
	for _, h := range s.hubs {
		out = append(out, HubStatus{
			Name: h.name, Role: "hub", Fingerprint: Fingerprint(h.ed),
			RTTMs: h.rtt.Milliseconds(), Proto: h.c.Protocol(),
		})
	}
	return out
}

func (s *Server) Dumps() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dumps
}

type HubStatus struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Self        bool   `json:"self,omitempty"`
	Fingerprint string `json:"fingerprint"`
	IPv6        string `json:"ipv6,omitempty"`
	IPv4        string `json:"ipv4,omitempty"`
	RTTMs       int64  `json:"rtt_ms,omitempty"`
	Proto       string `json:"proto,omitempty"`
	CloudSeed   bool   `json:"cloud_seed,omitempty"`
	Spokes      int    `json:"spokes,omitempty"`
	Peers       int    `json:"peers,omitempty"`
}

func values(m map[string]*session) []*session {
	out := make([]*session, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func listenUDP(host string, port int, v6only bool) (net.PacketConn, error) {
	cfg := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			if !v6only {
				return nil
			}
			var sockErr error
			_ = c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
			})
			return sockErr
		},
	}
	network := "udp"
	if v6only {
		network = "udp6"
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	if host == "" && v6only {
		addr = fmt.Sprintf("[::]:%d", port)
	}
	return cfg.ListenPacket(context.Background(), network, addr)
}

func listenTCP(host string, port int, v6only bool) (net.Listener, error) {
	cfg := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			if !v6only {
				return nil
			}
			var sockErr error
			_ = c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
			})
			return sockErr
		},
	}
	network := "tcp"
	if v6only {
		network = "tcp6"
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	if host == "" && v6only {
		addr = fmt.Sprintf("[::]:%d", port)
	}
	return cfg.Listen(context.Background(), network, addr)
}

func hostOnly(a net.Addr) string {
	if a == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	return host
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func edEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	ok := true
	for i := range a {
		ok = ok && a[i] == b[i]
	}
	return ok
}

func setReadDeadline(c Conn, d time.Duration) error {
	type deadliner interface{ SetReadDeadline(time.Time) error }
	if f, ok := c.(*frameConn); ok {
		if x, ok := f.rw.(deadliner); ok {
			return x.SetReadDeadline(time.Now().Add(d))
		}
	}
	return nil
}

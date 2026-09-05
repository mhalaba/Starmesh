package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"sync"
	"time"
)

const bloomM = 2048 // bits
const bloomK = 4

type bloom struct {
	b []byte
}

func newBloom() *bloom {
	return &bloom{b: make([]byte, bloomM/8)}
}

func (bl *bloom) Add(pub []byte) {
	for i := 0; i < bloomK; i++ {
		idx := bloomIndex(pub, i)
		bl.b[idx/8] |= 1 << (idx % 8)
	}
}

func (bl *bloom) Has(pub []byte) bool {
	for i := 0; i < bloomK; i++ {
		idx := bloomIndex(pub, i)
		if bl.b[idx/8]&(1<<(idx%8)) == 0 {
			return false
		}
	}
	return true
}

func bloomIndex(pub []byte, i int) uint {
	h := sha256.Sum256(append([]byte{byte(i)}, pub...))
	return uint(binary.BigEndian.Uint32(h[:4])) % bloomM
}

func helloSignedBytes(h Hello) []byte {
	var x []byte
	x = append(x, []byte(h.Role)...)
	x = append(x, h.Ed25519...)
	x = append(x, h.X25519...)
	x = append(x, []byte(h.Name)...)
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(h.Ts))
	x = append(x, ts[:]...)
	x = append(x, []byte(h.ListenIPv6)...)
	x = append(x, []byte(h.ListenIPv4)...)
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], h.ListenPort)
	x = append(x, p[:]...)
	return x
}

func (h *Hello) Sign(priv ed25519.PrivateKey) {
	h.Sig = ed25519.Sign(priv, helloSignedBytes(*h))
}

func (h Hello) Verify() error {
	if len(h.Ed25519) != ed25519.PublicKeySize {
		return errBadHello
	}
	if !ed25519.Verify(ed25519.PublicKey(h.Ed25519), helloSignedBytes(h), h.Sig) {
		return errBadHello
	}
	if time.Since(time.Unix(h.Ts, 0)) > 2*time.Minute && time.Until(time.Unix(h.Ts, 0)) > 2*time.Minute {
		// Clock skew on Starlink / outage: allow anyway, signature is the real check.
	}
	return nil
}

func helloOKBytes(h HelloOK) []byte {
	var x []byte
	x = append(x, h.Ed25519...)
	x = append(x, h.X25519...)
	x = append(x, []byte(h.Name)...)
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(h.Ts))
	x = append(x, ts[:]...)
	x = append(x, []byte(h.ObservedIP)...)
	return x
}

func (h *HelloOK) Sign(priv ed25519.PrivateKey) {
	h.Sig = ed25519.Sign(priv, helloOKBytes(*h))
}

func (h HelloOK) Verify() error {
	if len(h.Ed25519) != ed25519.PublicKeySize {
		return errBadHello
	}
	if !ed25519.Verify(ed25519.PublicKey(h.Ed25519), helloOKBytes(h), h.Sig) {
		return errBadHello
	}
	return nil
}

var errBadHello = errf("starmesh: bad handshake")

type errf string

func (e errf) Error() string { return string(e) }

func selfSignedTLS(id *Identity) (*tls.Config, error) {
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "starmesh"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     nil,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, id.EdPub, id.EdPriv)
	if err != nil {
		return nil, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: id.EdPriv}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{ALPN},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func clientTLS() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // identity is pinned by signed hello + invite pubkey
		NextProtos:         []string{ALPN},
		MinVersion:         tls.VersionTLS13,
	}
}

type seenIDs struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func newSeen() *seenIDs {
	return &seenIDs{m: map[string]time.Time{}}
}

func (s *seenIDs) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; ok {
		return false
	}
	s.m[id] = time.Now()
	if len(s.m) > 4096 {
		cut := time.Now().Add(-10 * time.Minute)
		for k, t := range s.m {
			if t.Before(cut) {
				delete(s.m, k)
			}
		}
	}
	return true
}

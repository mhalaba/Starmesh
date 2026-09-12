package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func startDevHub(t *testing.T, name string, peers ...string) *Server {
	t.Helper()
	id, err := GenerateIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(id, ServerConfig{
		Name:     name,
		Host:     "::1",
		Port:     0,
		IPv6Only: true,
		PublicV6: net.ParseIP("::1"),
		Dev:      true,
		Capture:  true,
		Peers:    peers,
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s
}

func startSpoke(t *testing.T, name string, invites []string) (*Spoke, *Identity, *bytes.Buffer) {
	t.Helper()
	id, err := GenerateIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	var mu sync.Mutex
	sp := NewSpoke(id, SpokeConfig{
		Name:    name,
		Invites: invites,
		Capture: true,
		OnMessage: func(from, n, text string) {
			mu.Lock()
			fmt.Fprintf(&got, "%s:%s\n", n, text)
			mu.Unlock()
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(sp.Stop)
	go func() { _ = sp.Run(ctx) }()
	return sp, id, &got
}

func waitPeer(t *testing.T, sp *Spoke, name string) peerInfo {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range sp.Peers() {
			if p.Name == name && p.X != [32]byte{} {
				return p
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("peer %s not seen", name)
	return peerInfo{}
}

func waitHub(t *testing.T, sp *Spoke) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if sp.ConnectedHub() != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("spoke never connected")
}

func inviteBlob(t *testing.T, s *Server) string {
	t.Helper()
	blob, err := s.Invite().Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(blob, ".") && strings.Contains(strings.ToLower(blob), "com") {
		t.Fatal("dns leakage")
	}
	return blob
}

func TestAcceptanceTwoSpokesIPv6OnlyHub(t *testing.T) {
	hub := startDevHub(t, "OSP Nadarzyn")
	blob := inviteBlob(t, hub)

	alice, _, aliceLog := startSpoke(t, "alice", []string{blob})
	bob, _, bobLog := startSpoke(t, "bob", []string{blob})
	waitHub(t, alice)
	waitHub(t, bob)
	ap := waitPeer(t, alice, "bob")
	bp := waitPeer(t, bob, "alice")

	if err := alice.Send(ap.Ed, ap.X, "ping-from-alice"); err != nil {
		t.Fatal(err)
	}
	if err := bob.Send(bp.Ed, bp.X, "pong-from-bob"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(bobLog.String(), "ping-from-alice") && strings.Contains(aliceLog.String(), "pong-from-bob") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(bobLog.String(), "ping-from-alice") {
		t.Fatalf("bob missed chat: %q", bobLog.String())
	}
	if !strings.Contains(aliceLog.String(), "pong-from-bob") {
		t.Fatalf("alice missed chat: %q", aliceLog.String())
	}

	// IPv4 inbound is impossible: hub is V6ONLY.
	d := net.Dialer{Timeout: time.Second}
	if c, err := d.Dial("tcp4", fmt.Sprintf("127.0.0.1:%d", hub.cfg.Port)); err == nil {
		_ = c.Close()
		t.Fatal("ipv4 dial to ipv6-only hub must fail (simulated CGNAT / no inbound IPv4)")
	}

	// Ciphertext only: hub dumps must not contain the plaintext.
	secret := []byte("ping-from-alice")
	for _, dump := range hub.Dumps() {
		if bytes.Contains(dump, secret) {
			t.Fatal("plaintext leaked on hub wire dump")
		}
	}
	if d := alice.Dump(); len(d) > 0 && bytes.Contains(d, secret) {
		// The spoke writes the envelope JSON which includes ciphertext, not plaintext.
		// If plaintext appears, Seal failed to encrypt.
		t.Fatal("plaintext leaked on spoke dump")
	}
}

func TestHubStartedAfterOutageInviteIsEnough(t *testing.T) {
	// No pre-existing cloud. Hub appears after "outage"; spokes get the invite.
	aliceID, _ := GenerateIdentity("alice")
	var got string
	sp := NewSpoke(aliceID, SpokeConfig{Name: "alice"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = sp.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)
	if sp.ConnectedHub() != nil {
		t.Fatal("must not connect without a hub")
	}

	hub := startDevHub(t, "field-hub")
	blob := inviteBlob(t, hub)
	_ = sp.AddInvite(blob)
	waitHub(t, sp)
	_ = got
}

func TestFederationTwoHubs(t *testing.T) {
	hubA := startDevHub(t, "hub-A")
	blobA := inviteBlob(t, hubA)
	hubB := startDevHub(t, "hub-B", blobA)
	blobB := inviteBlob(t, hubB)

	alice, _, aliceLog := startSpoke(t, "alice", []string{blobA})
	bob, _, bobLog := startSpoke(t, "bob", []string{blobB})
	waitHub(t, alice)
	waitHub(t, bob)
	ap := waitPeer(t, alice, "bob")
	if err := alice.Send(ap.Ed, ap.X, "via-federation"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(bobLog.String(), "via-federation") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("federated chat failed alice=%q bob=%q", aliceLog.String(), bobLog.String())
}

func TestFailoverUsesCache(t *testing.T) {
	hub1 := startDevHub(t, "hub-1")
	hub2 := startDevHub(t, "hub-2")
	blob1 := inviteBlob(t, hub1)
	blob2 := inviteBlob(t, hub2)

	alice, _, _ := startSpoke(t, "alice", []string{blob1, blob2})
	bob, _, bobLog := startSpoke(t, "bob", []string{blob1, blob2})
	waitHub(t, alice)
	waitHub(t, bob)
	waitPeer(t, alice, "bob")

	hub1.Stop()
	time.Sleep(2 * time.Second)
	waitHub(t, alice)
	waitHub(t, bob)
	ap := waitPeer(t, alice, "bob")
	if err := alice.Send(ap.Ed, ap.X, "after-failover"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(bobLog.String(), "after-failover") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("failover chat failed: %q", bobLog.String())
}

func TestEnvelopeJSONHasNoPlaintextOnMarshal(t *testing.T) {
	a, _ := GenerateIdentity("a")
	b, _ := GenerateIdentity("b")
	secret := []byte("classified-body-xyz")
	env, err := Seal(a, b.XPub, b.EdPub, EnvChat, secret)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(env)
	if bytes.Contains(raw, secret) {
		t.Fatal("plaintext in envelope JSON")
	}
	if env.Type != EnvChat {
		t.Fatal(env.Type)
	}
}

func TestCommunitySignVerify(t *testing.T) {
	root, err := GenerateIdentity("root")
	if err != nil {
		t.Fatal(err)
	}
	hubID, _ := GenerateIdentity("OSP")
	f, err := SignCommunity(root.EdPriv, []CachedHub{{
		Name:    "OSP Nadarzyn",
		Ed25519: hexBytes(hubID.EdPub),
		IPv6:    "2a0d:3344:1::2",
		Port:    4433,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Verify(root.EdPub); err != nil {
		t.Fatal(err)
	}
	f.Sig[0] ^= 1
	if err := f.Verify(root.EdPub); err == nil {
		t.Fatal("tamper")
	}
}

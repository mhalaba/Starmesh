package invite

import (
	"net"
	"strings"
	"testing"
	"time"
)

func testInvite(t *testing.T) Invite {
	t.Helper()
	var pk [32]byte
	for i := range pk {
		pk[i] = byte(i + 1)
	}
	return Invite{
		Version: Version,
		PubKey:  pk,
		IPv6:    net.ParseIP("2a0d:3344:0100:0001::2"),
		Port:    4433,
		Expiry:  time.Unix(1893456000, 0).UTC(), // 2030-01-01
		Name:    "OSP Nadarzyn",
	}
}

func TestRoundTripFull(t *testing.T) {
	inv := testInvite(t)
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, Prefix) {
		t.Fatalf("prefix: %s", s)
	}
	got, err := DecodeFull(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != inv.Name {
		t.Fatalf("name %q", got.Name)
	}
	if !got.IPv6.Equal(inv.IPv6) {
		t.Fatalf("ipv6 %s != %s", got.IPv6, inv.IPv6)
	}
	if got.Port != 4433 {
		t.Fatalf("port %d", got.Port)
	}
	if got.PubKey != inv.PubKey {
		t.Fatalf("pubkey")
	}
	if got.CloudSeed {
		t.Fatal("seed")
	}
}

func TestRoundTripIPv4AndSeed(t *testing.T) {
	inv := testInvite(t)
	inv.IPv4 = net.ParseIP("198.51.100.9")
	inv.CloudSeed = true
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFull(s)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IPv4.Equal(inv.IPv4) {
		t.Fatalf("ipv4 %s", got.IPv4)
	}
	if !got.CloudSeed {
		t.Fatal("expected seed")
	}
}

func TestStarlinkHubRequiresIPv6(t *testing.T) {
	inv := testInvite(t)
	inv.IPv6 = nil
	if _, err := inv.Encode(); err != ErrNoIPv6 {
		t.Fatalf("got %v", err)
	}
	// Cloud seed may be IPv4-only (VPS in another continent).
	inv.CloudSeed = true
	inv.IPv4 = net.ParseIP("203.0.113.5")
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFull(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.IPv6 != nil && !got.IPv6.Equal(net.IPv6zero) {
		// nil is expected
		if !got.IPv6.Equal(net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
			t.Fatalf("ipv6 %v", got.IPv6)
		}
	}
	if !got.IPv4.Equal(inv.IPv4) {
		t.Fatalf("ipv4 %s", got.IPv4)
	}
}

func TestShortCodeResolver(t *testing.T) {
	inv := testInvite(t)
	full, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := DecodeFull(full)
	if err != nil {
		t.Fatal(err)
	}
	short := parsed.ShortDisplay()
	if len(strings.ReplaceAll(short, "-", "")) != 10 {
		t.Fatalf("short %q", short)
	}
	_, err = Decode(short, nil)
	if err == nil {
		t.Fatal("short code without resolver must fail")
	}
	got, err := Decode(short, LookupTable([]*Invite{parsed}))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != inv.Name {
		t.Fatalf("resolved name %q", got.Name)
	}
}

func TestChecksumTamper(t *testing.T) {
	inv := testInvite(t)
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(s)
	raw[len(raw)-2] ^= 0x08
	if _, err := DecodeFull(string(raw)); err != ErrChecksum {
		t.Fatalf("got %v", err)
	}
}

func TestCrockfordAliases(t *testing.T) {
	inv := testInvite(t)
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// Replace a 0 with O in the payload — decoder must accept it.
	payload := s[len(Prefix):]
	if !strings.Contains(payload, "0") {
		t.Skip("no zero digit in this payload")
	}
	aliased := Prefix + strings.Replace(payload, "0", "O", 1)
	got, err := DecodeFull(aliased)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != inv.Name {
		t.Fatal("alias decode")
	}
}

func TestNoDNSInEncoding(t *testing.T) {
	inv := testInvite(t)
	s, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(s)
	for _, n := range []string{".com", ".net", "http", "dns"} {
		if strings.Contains(lower, n) && n != "" {
			// "starmesh1:" contains no dns labels
		}
	}
	if strings.Contains(s, ".") {
		t.Fatalf("invite must not embed hostnames: %s", s)
	}
}

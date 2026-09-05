package hub

import (
	"net"
	"strings"
	"testing"
)

func TestCGNATDetection(t *testing.T) {
	if !isCGNAT(net.ParseIP("100.64.0.1")) {
		t.Fatal("100.64/10 is CGNAT")
	}
	if !isCGNAT(net.ParseIP("100.127.255.254")) {
		t.Fatal("100.127 is CGNAT")
	}
	if isCGNAT(net.ParseIP("100.63.255.255")) {
		t.Fatal("100.63 is not CGNAT")
	}
	if isCGNAT(net.ParseIP("198.51.100.9")) {
		t.Fatal("TEST-NET-2 is public")
	}
	if publicIPv4([]net.IP{net.ParseIP("192.168.1.2"), net.ParseIP("100.64.1.1")}) != nil {
		t.Fatal("must not claim RFC1918 or CGNAT as public IPv4")
	}
	pub := publicIPv4([]net.IP{net.ParseIP("192.168.1.2"), net.ParseIP("198.51.100.9")})
	if pub == nil || !pub.Equal(net.ParseIP("198.51.100.9")) {
		t.Fatalf("got %v", pub)
	}
}

func TestProbeRefuseWithoutIPv6(t *testing.T) {
	c := Probe(ProbeOpts{Port: 0, Dev: false})
	// CI boxes often have no global IPv6. Dev=false must not claim hub
	// unless 2000::/3 is present.
	hasGlobal := false
	for _, ip := range c.GlobalIPv6 {
		if isGlobalIPv6(ip) {
			hasGlobal = true
		}
	}
	if !hasGlobal && c.CanBeHub {
		t.Fatal("must not become hub without global IPv6")
	}
	if !hasGlobal && !strings.HasPrefix(c.Reason, HubRefuse) {
		t.Fatalf("reason %q", c.Reason)
	}
}

func TestProbeDevAllowsLoopback(t *testing.T) {
	c := Probe(ProbeOpts{Port: 0, Dev: true})
	if !c.CanBeHub {
		t.Fatalf("dev mode should allow loopback hub: %+v", c)
	}
}

func TestObservedIPv4MustMatchLocal(t *testing.T) {
	c := Probe(ProbeOpts{
		Dev:         true,
		ObserveIPv4: net.ParseIP("198.51.100.50"),
	})
	if c.ClaimIPv4 {
		t.Fatal("must not claim IPv4 when observed address is not local (CGNAT)")
	}
}

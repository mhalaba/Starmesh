package hub

import (
	"fmt"
	"net"
	"strings"
)

const HubRefuse = "This dish cannot be a hub. Put the app on a PC behind a bypass router or use a Priority public IP."

// Cap is the result of a local capability probe. No DNS.
type Cap struct {
	GlobalIPv6     []net.IP `json:"global_ipv6"`
	LocalIPv4      []net.IP `json:"local_ipv4"`
	PublicIPv4     net.IP   `json:"public_ipv4,omitempty"`
	BehindCGNAT    bool     `json:"behind_cgnat"`
	CanBindUDP     bool     `json:"can_bind_udp"`
	BindPort       int      `json:"bind_port"`
	BindError      string   `json:"bind_error,omitempty"`
	CanBeHub       bool     `json:"can_be_hub"`
	ClaimIPv4      bool     `json:"claim_ipv4"`
	Reason         string   `json:"reason,omitempty"`
	DevRelaxed     bool     `json:"dev_relaxed,omitempty"`
	HasDefaultIPv6 bool     `json:"has_default_ipv6"`
}

type ProbeOpts struct {
	Port        int
	Dev         bool // allow loopback / ULA so labs and CI can become a hub
	ObserveIPv4 net.IP
}

func Probe(opts ProbeOpts) Cap {
	if opts.Port == 0 {
		opts.Port = 4433
	}
	c := Cap{BindPort: opts.Port, DevRelaxed: opts.Dev}

	ifaces, _ := net.Interfaces()
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 && !opts.Dev {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ip := ipFromAddr(a)
			if ip == nil {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				c.LocalIPv4 = append(c.LocalIPv4, ip4)
				continue
			}
			if isGlobalIPv6(ip) || (opts.Dev && isUsableDevIPv6(ip)) {
				c.GlobalIPv6 = append(c.GlobalIPv6, ip)
			}
		}
	}
	if opts.Dev {
		c.GlobalIPv6 = uniqueIPs(append(c.GlobalIPv6, net.ParseIP("::1")))
	}

	c.CanBindUDP, c.BindError = canBindUDP(opts.Port, len(c.GlobalIPv6) > 0)

	c.BehindCGNAT = anyCGNAT(c.LocalIPv4)
	pub := publicIPv4(c.LocalIPv4)
	if opts.ObserveIPv4 != nil {
		obs := opts.ObserveIPv4.To4()
		if pub != nil && obs != nil && pub.Equal(obs) {
			c.PublicIPv4 = pub
		} else if obs != nil && !isPrivateIPv4(obs) && !isCGNAT(obs) {
			// Observed by a peer. Only claim if a local address matches
			// (WAN IP == observed IPv4). Otherwise we are behind NAT.
			if hasIP(c.LocalIPv4, obs) {
				c.PublicIPv4 = obs
			} else {
				c.BehindCGNAT = true
			}
		}
	} else if pub != nil {
		c.PublicIPv4 = pub
	}

	c.ClaimIPv4 = c.PublicIPv4 != nil && !c.BehindCGNAT
	c.HasDefaultIPv6 = len(c.GlobalIPv6) > 0

	switch {
	case !c.CanBindUDP:
		c.CanBeHub = false
		c.Reason = HubRefuse + " (cannot bind UDP :" + itoa(opts.Port) + ")"
	case len(c.GlobalIPv6) == 0:
		c.CanBeHub = false
		c.Reason = HubRefuse
	default:
		c.CanBeHub = true
		if c.ClaimIPv4 {
			c.Reason = "hub-capable: global IPv6 + public IPv4"
		} else {
			c.Reason = "hub-capable: global IPv6 (no public IPv4 — CGNAT or RFC1918; IPv4 inbound will not be advertised)"
		}
	}
	return c
}

func (c Cap) PickIPv6() net.IP {
	for _, ip := range c.GlobalIPv6 {
		if isGlobalIPv6(ip) {
			return ip
		}
	}
	if len(c.GlobalIPv6) > 0 {
		return c.GlobalIPv6[0]
	}
	return nil
}

func canBindUDP(port int, wantv6 bool) (bool, string) {
	network := "udp"
	addr := fmt.Sprintf(":%d", port)
	if wantv6 {
		network = "udp6"
		addr = fmt.Sprintf("[::]:%d", port)
	}
	pc, err := net.ListenPacket(network, addr)
	if err != nil {
		// Fall back to an ephemeral port to distinguish "port busy" from "no UDP".
		if strings.Contains(err.Error(), "address already in use") {
			return true, ""
		}
		pc2, err2 := net.ListenPacket("udp6", "[::]:0")
		if err2 != nil {
			return false, err.Error()
		}
		_ = pc2.Close()
		return true, "port busy: " + err.Error()
	}
	_ = pc.Close()
	return true, ""
}

func ipFromAddr(a net.Addr) net.IP {
	switch v := a.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		host, _, err := net.SplitHostPort(a.String())
		if err != nil {
			return net.ParseIP(a.String())
		}
		return net.ParseIP(host)
	}
}

func isGlobalIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return false
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	// Global unicast 2000::/3
	return ip[0]&0xe0 == 0x20
}

func isUsableDevIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() // ULA fc00::/7 is IsPrivate in Go 1.17+
}

func isCGNAT(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}

func isPrivateIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsPrivate() {
		return true
	}
	return isCGNAT(ip4)
}

func anyCGNAT(ips []net.IP) bool {
	for _, ip := range ips {
		if isCGNAT(ip) {
			return true
		}
	}
	return false
}

func publicIPv4(ips []net.IP) net.IP {
	for _, ip := range ips {
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		if !isPrivateIPv4(ip4) && !ip4.IsMulticast() && !ip4.IsUnspecified() {
			return ip4
		}
	}
	return nil
}

func hasIP(list []net.IP, want net.IP) bool {
	for _, ip := range list {
		if ip.Equal(want) {
			return true
		}
	}
	return false
}

func uniqueIPs(in []net.IP) []net.IP {
	seen := map[string]struct{}{}
	var out []net.IP
	for _, ip := range in {
		k := ip.String()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, ip)
	}
	return out
}

func itoa(i int) string { return fmt.Sprintf("%d", i) }

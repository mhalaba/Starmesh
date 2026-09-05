package hub

import (
	"log/slog"
	"net"
)

// RAReport is what a would-be hub needs from IPv6 Router Advertisements.
// The stock Starlink router in bypass mode is no longer the LAN router —
// the mini-PC (or third-party router) must keep the delegated /56 alive
// with radvd/systemd-networkd. This process does not send ICMPv6 RAs
// unless the operator runs radvd (see deploy/radvd.conf).
type RAReport struct {
	GlobalPrefixes []string `json:"global_prefixes"`
	Advice         string   `json:"advice"`
}

func ProbeRA() RAReport {
	r := RAReport{}
	ifaces, _ := net.Interfaces()
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ip, ok := a.(*net.IPNet)
			if !ok || ip.IP.To4() != nil {
				continue
			}
			if isGlobalIPv6(ip.IP) {
				ones, _ := ip.Mask.Size()
				r.GlobalPrefixes = append(r.GlobalPrefixes, ip.IP.String()+"/"+itoa(ones))
			}
		}
	}
	if len(r.GlobalPrefixes) == 0 {
		r.Advice = "No global IPv6 prefix on this host. Enable bypass mode, confirm the dish delegated a /56, and run radvd (deploy/radvd.conf) so LAN devices keep global addresses."
	} else {
		r.Advice = "Global IPv6 prefix present. Keep radvd/systemd-networkd advertising it; the stock Starlink router will not, once in bypass."
	}
	return r
}

func LogRA(log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	r := ProbeRA()
	log.Info("ipv6 prefix", "prefixes", r.GlobalPrefixes, "advice", r.Advice)
}

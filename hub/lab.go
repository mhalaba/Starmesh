package hub

import (
	"net"
	"os"
)

// PickLabIPv4 returns the first RFC1918 IPv4 on this host that is not
// loopback or CGNAT. Used only with --dev so a LAN spoke can dial the
// lab hub over IPv4 when the Pi has no global IPv6.
func PickLabIPv4() net.IP {
	ifaces, _ := net.Interfaces()
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ip := ipFromAddr(a)
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() || isCGNAT(ip4) {
				continue
			}
			if ip4.IsPrivate() {
				return ip4
			}
		}
	}
	return nil
}

// StdinIsTTY is false under systemd, pipes, and </dev/null>.
func StdinIsTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

// APIIsLoopback reports whether addr is loopback-only (empty host or 127.0.0.1/::1).
func APIIsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr == "127.0.0.1" || addr == "::1" || addr == "localhost"
	}
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

package hub

import (
	"fmt"
	"io"
	"strings"

	"github.com/mhalaba/Starmesh/invite"
	"rsc.io/qr"
)

func PrintInvite(w io.Writer, inv *invite.Invite) error {
	blob, err := inv.Encode()
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "Starmesh hub invite (no DNS)")
	fmt.Fprintf(w, "  name:    %s\n", inv.Name)
	fmt.Fprintf(w, "  short:   %s\n", inv.ShortDisplay())
	fmt.Fprintf(w, "  ipv6:    %s\n", inv.AddrIPv6())
	if inv.IPv4 != nil {
		fmt.Fprintf(w, "  ipv4:    %s\n", inv.AddrIPv4())
	} else {
		fmt.Fprintln(w, "  ipv4:    (none — CGNAT / not claimed)")
	}
	if inv.CloudSeed {
		fmt.Fprintln(w, "  class:   cloud seed (last resort)")
	}
	fmt.Fprintf(w, "  blob:    %s\n", blob)
	fmt.Fprintln(w)
	code, err := qr.Encode(blob, qr.M)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, qrASCII(code))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Family phones on this LAN can also pick it up via mDNS/LAN beacon.")
	fmt.Fprintln(w, "Short code works only if the hub is already in cache or the community list.")
	return nil
}

func qrASCII(c *qr.Code) string {
	var b strings.Builder
	// quiet zone
	n := c.Size
	black, white := "██", "  "
	for y := -1; y <= n; y++ {
		for x := -1; x <= n; x++ {
			if x < 0 || y < 0 || x >= n || y >= n || !c.Black(x, y) {
				b.WriteString(white)
			} else {
				b.WriteString(black)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

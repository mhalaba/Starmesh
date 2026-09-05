package hub

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"time"
)

// LANBeacon is mDNS-class discovery for devices on the SAME Starlink LAN
// (family phones, a laptop next to the hub). It never leaves the broadcast
// domain and carries the full invite so there is no DNS.
const lanPort = 53535

type lanBeacon struct {
	log *slog.Logger
}

func AdvertiseLAN(ctx context.Context, payload string, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	go announce(ctx, "udp6", "[ff02::1]:"+itoa(lanPort), payload, log)
	go announce(ctx, "udp4", "224.0.0.251:"+itoa(lanPort), payload, log)
}

func BrowseLAN(ctx context.Context, log *slog.Logger) <-chan string {
	ch := make(chan string, 8)
	go listenLAN(ctx, "udp6", "[::]:"+itoa(lanPort), ch, log)
	go listenLAN(ctx, "udp4", "0.0.0.0:"+itoa(lanPort), ch, log)
	return ch
}

func announce(ctx context.Context, network, addr, payload string, log *slog.Logger) {
	udp, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return
	}
	c, err := net.DialUDP(network, nil, udp)
	if err != nil {
		log.Debug("lan announce", "net", network, "err", err)
		return
	}
	defer c.Close()
	msg := []byte("starmesh-lan " + payload)
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	_, _ = c.Write(msg)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = c.Write(msg)
		}
	}
}

func listenLAN(ctx context.Context, network, addr string, ch chan<- string, log *slog.Logger) {
	udp, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return
	}
	c, err := net.ListenUDP(network, udp)
	if err != nil {
		log.Debug("lan browse", "net", network, "err", err)
		return
	}
	defer c.Close()
	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := c.ReadFrom(buf)
		if err != nil {
			continue
		}
		s := string(buf[:n])
		if !strings.HasPrefix(s, "starmesh-lan ") {
			continue
		}
		inv := strings.TrimSpace(strings.TrimPrefix(s, "starmesh-lan "))
		select {
		case ch <- inv:
		default:
		}
	}
}

// Meshtastic is an MVP stub. A later build can send the invite (it fits in
// a Meshtastic text frame) as a LoRa beacon.
type Meshtastic interface {
	BroadcastInvite(code string) error
	OnInvite(func(code string))
}

type NoopMeshtastic struct{}

func (NoopMeshtastic) BroadcastInvite(string) error { return nil }
func (NoopMeshtastic) OnInvite(func(string))        {}

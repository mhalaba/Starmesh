package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mhalaba/Starmesh/hub"
	"github.com/mhalaba/Starmesh/invite"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	var err error
	switch os.Args[1] {
	case "hub":
		err = cmdHub(os.Args[2:])
	case "spoke":
		err = cmdSpoke(os.Args[2:])
	case "probe":
		err = cmdProbe(os.Args[2:])
	case "become-hub":
		err = cmdBecomeHub(os.Args[2:])
	case "invite":
		err = cmdInvite(os.Args[2:])
	case "identity":
		err = cmdIdentity(os.Args[2:])
	case "community":
		err = cmdCommunity(os.Args[2:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `starmesh — Starlink hub-and-spoke overlay (no DNS, no inbound IPv4)

Commands:
  probe          capability check (IPv6, UDP bind, CGNAT)
  become-hub     probe + start hub, print QR / 10-char invite
  hub            run as hub (Starlink station or cloud seed)
  spoke          outbound-only client; chat through a hub
  invite         encode / decode invite blobs
  identity       show this install's Ed25519 fingerprint
  community      sign a pre-provisioned hub list

Defaults: UDP/QUIC :4433, TCP/TLS fallback, 15s Starlink timeouts.
Cloud seeds are last resort. Spokes never listen.
`)
}

func homeFlag(fs *flag.FlagSet) *string {
	def := hub.DefaultHome()
	return fs.String("home", def, "state directory")
}

func cmdProbe(args []string) error {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	dev := fs.Bool("dev", false, "treat loopback/ULA as hub-capable (lab)")
	port := fs.Int("port", invite.DefaultPort, "UDP port to probe")
	_ = fs.Parse(args)
	c := hub.Probe(hub.ProbeOpts{Port: *port, Dev: *dev})
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(c)
	ra := hub.ProbeRA()
	fmt.Println("ra:", ra.Advice)
	if !c.CanBeHub {
		fmt.Println(c.Reason)
		return fmt.Errorf("not hub-capable")
	}
	return nil
}

func cmdBecomeHub(args []string) error {
	fs := flag.NewFlagSet("become-hub", flag.ExitOnError)
	dev := fs.Bool("dev", false, "allow loopback/ULA hubs")
	name := fs.String("name", "", "hub name (shown on the banner)")
	port := fs.Int("port", invite.DefaultPort, "listen port")
	home := homeFlag(fs)
	_ = fs.Parse(args)
	c := hub.Probe(hub.ProbeOpts{Port: *port, Dev: *dev})
	if !c.CanBeHub {
		fmt.Fprintln(os.Stderr, c.Reason)
		return fmt.Errorf("become-hub refused")
	}
	return runHub(*home, *name, *port, *dev, false, c, nil, "127.0.0.1:7780")
}

func cmdHub(args []string) error {
	fs := flag.NewFlagSet("hub", flag.ExitOnError)
	dev := fs.Bool("dev", false, "allow loopback/ULA")
	name := fs.String("name", "", "hub name")
	port := fs.Int("port", invite.DefaultPort, "listen port")
	seed := fs.Bool("cloud-seed", false, "mark as last-resort VPS seed")
	peer := fs.String("peer", "", "peer hub invite (repeat via comma)")
	force := fs.Bool("force", false, "skip capability refuse (still will not claim CGNAT IPv4)")
	home := homeFlag(fs)
	api := fs.String("api", "127.0.0.1:7780", "loopback JSON API for the Flutter UI (empty to disable)")
	_ = fs.Parse(args)
	c := hub.Probe(hub.ProbeOpts{Port: *port, Dev: *dev})
	if !c.CanBeHub && !*force && !*seed {
		fmt.Fprintln(os.Stderr, c.Reason)
		return fmt.Errorf("hub refused")
	}
	var peers []string
	if *peer != "" {
		peers = strings.Split(*peer, ",")
	}
	return runHub(*home, *name, *port, *dev, *seed, c, peers, *api)
}

func runHub(home, name string, port int, dev, seed bool, c hub.Cap, peers []string, apiAddr string) error {
	id, err := hub.LoadOrCreateIdentity(home, name)
	if err != nil {
		return err
	}
	if name == "" {
		name = id.Name
	}
	if name == "" {
		name = "starmesh-hub"
	}
	cfg := hub.ServerConfig{
		Name:      name,
		Port:      port,
		IPv6Only:  !c.ClaimIPv4,
		ClaimIPv4: c.ClaimIPv4,
		PublicV6:  c.PickIPv6(),
		PublicV4:  c.PublicIPv4,
		CloudSeed: seed,
		Dev:       dev,
		Peers:     peers,
	}
	if dev && cfg.PublicV6 == nil {
		cfg.PublicV6 = net.ParseIP("::1")
		cfg.Host = "::1"
		cfg.IPv6Only = true
	}
	srv := hub.NewServer(id, cfg)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := srv.Start(ctx); err != nil {
		return err
	}
	hub.LogRA(slog.Default())
	inv := srv.Invite()
	if err := hub.PrintInvite(os.Stdout, inv); err != nil {
		return err
	}
	blob, _ := inv.Encode()
	hub.AdvertiseLAN(ctx, blob, slog.Default())
	if apiAddr != "" {
		api := &hub.LocalAPI{
			Status: func() map[string]any {
				return map[string]any{
					"role":   "hub",
					"banner": fmt.Sprintf("Hub: %s (IPv6 Starlink)", name),
					"hubs":   srv.Status(),
					"invite": blob,
					"short":  inv.ShortDisplay(),
				}
			},
			Invite: func() string { return blob },
			QR:     func() string { return blob },
			Banner: func() string { return fmt.Sprintf("Hub: %s (IPv6 Starlink)", name) },
			Become: func() (string, error) { return "Already running as a hub: " + name, nil },
			Stop:   func() { srv.Stop(); stop() },
		}
		go func() {
			slog.Info("local api", "addr", apiAddr)
			_ = http.ListenAndServe(apiAddr, api.Handler())
		}()
	}
	fmt.Println("role: hub  (Ctrl-C to stop)")
	<-ctx.Done()
	srv.Stop()
	return nil
}

func cmdSpoke(args []string) error {
	fs := flag.NewFlagSet("spoke", flag.ExitOnError)
	name := fs.String("name", "", "display name")
	inv := fs.String("invite", "", "starmesh1: blob, QR payload, or 10-char code")
	ipv6 := fs.String("ipv6", "", "manual hub IPv6 (no DNS)")
	port := fs.Uint("port", invite.DefaultPort, "hub port")
	key := fs.String("key", "", "hub Ed25519 pubkey hex (with --ipv6)")
	to := fs.String("to", "", "peer name or fingerprint to chat with")
	community := fs.String("community", "", "signed community-hubs.json")
	seed := fs.String("seed", "", "cloud-seed invite (lowest priority)")
	mdns := fs.Bool("mdns", false, "browse LAN beacons on this Starlink LAN")
	apiAddr := fs.String("api", "127.0.0.1:7780", "loopback JSON API for the Flutter UI (empty to disable)")
	home := homeFlag(fs)
	_ = fs.Parse(args)

	id, err := hub.LoadOrCreateIdentity(*home, *name)
	if err != nil {
		return err
	}
	if *name == "" {
		*name = id.Name
	}
	cache := hub.OpenCache(hub.CachePath(*home))
	q := hub.OpenQueue(hub.QueuePath(*home), 1000)
	var comm []hub.CachedHub
	if *community != "" {
		f, err := hub.LoadCommunity(*community, nil)
		if err != nil {
			return err
		}
		comm = f.Hubs
	}
	var invites []string
	if *inv != "" {
		invites = append(invites, *inv)
	}
	if *seed != "" {
		invites = append(invites, *seed)
	}
	var manualKey []byte
	if *key != "" {
		var err error
		manualKey, err = hex.DecodeString(strings.TrimSpace(*key))
		if err != nil || len(manualKey) != 32 {
			return fmt.Errorf("--key must be 32-byte ed25519 hex")
		}
	}
	var banner string
	var localAPI *hub.LocalAPI
	sp := hub.NewSpoke(id, hub.SpokeConfig{
		Name:       *name,
		Invites:    invites,
		ManualIPv6: *ipv6,
		ManualPort: uint16(*port),
		ManualKey:  manualKey,
		Community:  comm,
		Cache:      cache,
		Queue:      q,
		OnMessage: func(from, n, text string) {
			fmt.Printf("\r<%s %s> %s\n> ", n, from, text)
			if localAPI != nil {
				localAPI.Push(from, n, text, false)
			}
		},
		OnStatus: func(s string) {
			banner = s
			fmt.Fprintln(os.Stderr, s)
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *mdns {
		go func() {
			for blob := range hub.BrowseLAN(ctx, slog.Default()) {
				sp.AddInvite(blob)
				fmt.Fprintln(os.Stderr, "lan invite:", blob[:min(40, len(blob))], "...")
			}
		}()
	}
	go func() { _ = sp.Run(ctx) }()

	if *apiAddr != "" {
		localAPI = &hub.LocalAPI{
			Status: func() map[string]any {
				m := map[string]any{"role": "spoke"}
				var hubs []map[string]any
				if h := sp.ConnectedHub(); h != nil {
					hubs = append(hubs, map[string]any{
						"name":        h.Name,
						"role":        "hub",
						"fingerprint": hub.Fingerprint(h.Ed25519),
						"cloud_seed":  h.CloudSeed,
						"proto":       "quic",
					})
				}
				m["hubs"] = hubs
				peers := []map[string]any{}
				for _, p := range sp.Peers() {
					peers = append(peers, map[string]any{
						"name":        p.Name,
						"fingerprint": hub.Fingerprint(p.Ed),
					})
				}
				m["peers"] = peers
				return m
			},
			Send:      func(to, text string) error { return sp.SendToName(to, text) },
			AddInvite: func(blob string) { sp.AddInvite(blob) },
			Become: func() (string, error) {
				c := hub.Probe(hub.ProbeOpts{Port: invite.DefaultPort})
				if !c.CanBeHub {
					return c.Reason, fmt.Errorf("not hub-capable")
				}
				return "This device can host a hub. Run: starmesh hub", nil
			},
			Banner: func() string { return banner },
		}
		go func() {
			slog.Info("local api", "addr", *apiAddr)
			_ = http.ListenAndServe(*apiAddr, localAPI.Handler())
		}()
	}

	fmt.Fprintf(os.Stderr, "spoke %s fp=%s\n", *name, id.Fingerprint())
	fmt.Fprintln(os.Stderr, "type messages; /peers  /to NAME  /status  /invite BLOB")
	go func() {
		in := bufio.NewScanner(os.Stdin)
		fmt.Print("> ")
		peer := *to
		for in.Scan() {
			line := strings.TrimSpace(in.Text())
			switch {
			case line == "":
			case line == "/peers":
				for _, p := range sp.Peers() {
					fmt.Printf("  %s  %s\n", p.Name, hub.Fingerprint(p.Ed))
				}
			case line == "/status":
				fmt.Println(banner)
				if h := sp.ConnectedHub(); h != nil {
					fmt.Println("hub", h.Name, hub.Fingerprint(h.Ed25519))
				} else {
					fmt.Println("No hub — queued")
				}
			case strings.HasPrefix(line, "/to "):
				peer = strings.TrimSpace(strings.TrimPrefix(line, "/to "))
				fmt.Println("to", peer)
			case strings.HasPrefix(line, "/invite "):
				sp.AddInvite(strings.TrimSpace(strings.TrimPrefix(line, "/invite ")))
				fmt.Println("invite queued for next dial")
			default:
				if peer == "" {
					if err := sp.SendToName("", line); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				} else if err := sp.SendToName(peer, line); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}
			fmt.Print("> ")
		}
	}()
	// Stay alive for the local API and signals even when stdin is closed
	// (the Flutter app drives a spoke daemon with no attached terminal).
	<-ctx.Done()
	return nil
}

func cmdInvite(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("invite encode|decode ...")
	}
	switch args[0] {
	case "decode":
		fs := flag.NewFlagSet("invite decode", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		raw := strings.Join(fs.Args(), "")
		if raw == "" {
			b, _ := io.ReadAll(os.Stdin)
			raw = string(b)
		}
		inv, err := invite.Decode(raw, nil)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(inv)
	case "encode":
		fs := flag.NewFlagSet("invite encode", flag.ExitOnError)
		name := fs.String("name", "", "hub name")
		ip6 := fs.String("ipv6", "", "hub IPv6")
		ip4 := fs.String("ipv4", "", "optional public IPv4")
		port := fs.Uint("port", invite.DefaultPort, "port")
		key := fs.String("key", "", "ed25519 pubkey hex")
		seed := fs.Bool("cloud-seed", false, "cloud seed")
		_ = fs.Parse(args[1:])
		inv := invite.Invite{Version: invite.Version, Name: *name, Port: uint16(*port), CloudSeed: *seed}
		if *ip6 != "" {
			inv.IPv6 = net.ParseIP(*ip6)
		}
		if *ip4 != "" {
			inv.IPv4 = net.ParseIP(*ip4)
		}
		raw, err := parseKey(*key)
		if err != nil {
			return err
		}
		copy(inv.PubKey[:], raw)
		blob, err := inv.Encode()
		if err != nil {
			return err
		}
		fmt.Println(blob)
		fmt.Println(inv.ShortDisplay())
		return nil
	default:
		return fmt.Errorf("invite encode|decode")
	}
}

func cmdIdentity(args []string) error {
	fs := flag.NewFlagSet("identity", flag.ExitOnError)
	name := fs.String("name", "", "set name if creating")
	home := homeFlag(fs)
	_ = fs.Parse(args)
	id, err := hub.LoadOrCreateIdentity(*home, *name)
	if err != nil {
		return err
	}
	fmt.Println("name", id.Name)
	fmt.Println("fp  ", id.Fingerprint())
	fmt.Println("ed  ", id.PubHex())
	fmt.Printf("x   %x\n", id.XPub[:])
	return nil
}

func cmdCommunity(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("community sign -in hubs.json -out community-hubs.json")
	}
	fs := flag.NewFlagSet("community", flag.ExitOnError)
	in := fs.String("in", "", "unsigned hubs json array")
	out := fs.String("out", "community-hubs.json", "signed output")
	home := homeFlag(fs)
	_ = fs.Parse(args[1:])
	id, err := hub.LoadOrCreateIdentity(filepath.Join(*home, "community-root"), "community-root")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var hubs []hub.CachedHub
	if err := json.Unmarshal(b, &hubs); err != nil {
		return err
	}
	f, err := hub.SignCommunity(id.EdPriv, hubs)
	if err != nil {
		return err
	}
	if err := f.Save(*out); err != nil {
		return err
	}
	fmt.Println("signed", *out, "root", id.PubHex())
	return nil
}

func parseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("missing --key")
	}
	out, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(out) != 32 {
		return nil, fmt.Errorf("ed25519 pubkey must be 32 bytes hex")
	}
	return out, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

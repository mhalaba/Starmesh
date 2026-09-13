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
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
	case "node":
		err = cmdNode(os.Args[2:])
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
  node           Pi/lab: hub + local chat identity + web panel (preferred)
  hub            run as hub (Starlink station or cloud seed)
  spoke          outbound-only client; chat through a hub
  invite         encode / decode invite blobs
  identity       show this install's Ed25519 fingerprint
  community      sign a pre-provisioned hub list

Defaults: UDP/QUIC :4433, TCP/TLS fallback, 15s Starlink timeouts.
Cloud seeds are last resort. Spokes never listen.
Operator panel: --api 0.0.0.0:7780 (web UI + JSON). Not the seed host.
`)
}

func homeFlag(fs *flag.FlagSet) *string {
	def := hub.DefaultHome()
	return fs.String("home", def, "state directory (hub and spoke on one host need different --home)")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
	return runHub(runOpts{
		home: *home, name: *name, port: *port, dev: *dev, cap: c,
		api: envOr("STARMESH_API", "127.0.0.1:7780"), token: os.Getenv("STARMESH_API_TOKEN"),
		operator: true,
	})
}

func cmdHub(args []string) error {
	fs := flag.NewFlagSet("hub", flag.ExitOnError)
	dev := fs.Bool("dev", false, "allow loopback/ULA + lab LAN IPv4")
	name := fs.String("name", envOr("STARMESH_NAME", ""), "hub name")
	port := fs.Int("port", invite.DefaultPort, "listen port")
	seed := fs.Bool("cloud-seed", false, "mark as last-resort VPS seed")
	peer := fs.String("peer", "", "peer hub invite (repeat via comma)")
	force := fs.Bool("force", false, "skip capability refuse (still will not claim CGNAT IPv4)")
	home := homeFlag(fs)
	api := fs.String("api", envOr("STARMESH_API", "127.0.0.1:7780"), "web panel + JSON API (empty to disable)")
	token := fs.String("api-token", os.Getenv("STARMESH_API_TOKEN"), "optional bearer token if --api is not loopback")
	operator := fs.Bool("operator", true, "local chat identity under --home/operator (needed for web/Flutter send)")
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
	return runHub(runOpts{
		home: *home, name: *name, port: *port, dev: *dev, seed: *seed, cap: c, peers: peers,
		api: *api, token: *token, operator: *operator && *api != "",
	})
}

func cmdNode(args []string) error {
	fs := flag.NewFlagSet("node", flag.ExitOnError)
	dev := fs.Bool("dev", false, "lab hub without global IPv6 (Raspberry Pi / no dish)")
	name := fs.String("name", envOr("STARMESH_NAME", ""), "hub + chat display name")
	port := fs.Int("port", invite.DefaultPort, "QUIC/TLS listen port")
	home := homeFlag(fs)
	api := fs.String("api", envOr("STARMESH_API", "0.0.0.0:7780"), "web panel bind (LAN: 0.0.0.0:7780)")
	token := fs.String("api-token", os.Getenv("STARMESH_API_TOKEN"), "optional token for the LAN panel")
	force := fs.Bool("force", false, "skip capability refuse")
	_ = fs.Parse(args)
	c := hub.Probe(hub.ProbeOpts{Port: *port, Dev: *dev})
	if !c.CanBeHub && !*force && !*dev {
		fmt.Fprintln(os.Stderr, c.Reason)
		return fmt.Errorf("node refused (need global IPv6 or --dev)")
	}
	return runHub(runOpts{
		home: *home, name: *name, port: *port, dev: *dev, cap: c,
		api: *api, token: *token, operator: true,
	})
}

type runOpts struct {
	home, name, api, token string
	port                   int
	dev, seed, operator    bool
	cap                    hub.Cap
	peers                  []string
}

func runHub(o runOpts) error {
	home, name, port, dev, seed, c, peers := o.home, o.name, o.port, o.dev, o.seed, o.cap, o.peers
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
	if dev {
		if cfg.PublicV6 == nil {
			cfg.PublicV6 = net.ParseIP("::1")
		}
		// Lab: listen on all interfaces so a LAN spoke can use RFC1918 IPv4.
		// Real (non-dev) hubs still never claim CGNAT/RFC1918 as public IPv4.
		cfg.Host = ""
		cfg.IPv6Only = false
		cfg.LabIPv4 = hub.PickLabIPv4()
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

	chat := &hub.ChatLog{}
	var op *hub.Spoke
	var opID *hub.Identity
	if o.operator {
		op, opID, err = hub.StartOperatorSpoke(ctx, hub.OperatorHome(home), name, []string{blob}, chat)
		if err != nil {
			return err
		}
		slog.Info("operator spoke", "name", opID.Name, "fp", opID.Fingerprint(), "home", hub.OperatorHome(home))
	}

	started := time.Now()
	kind := "IPv6 Starlink"
	if dev {
		kind = "lab --dev"
	} else if seed {
		kind = "cloud seed"
	}
	banner := fmt.Sprintf("Hub: %s (%s)", name, kind)
	role := "hub"
	if o.operator {
		role = "node"
	}
	hub.ServeLocalAPI(o.api, &hub.LocalAPI{
		Token:   o.token,
		Started: started,
		Status: func() map[string]any {
			out := map[string]any{
				"role":       role,
				"name":       name,
				"banner":     banner,
				"hubs":       srv.Status(),
				"invite":     blob,
				"short":      inv.ShortDisplay(),
				"dev":        dev,
				"fingerprint": id.Fingerprint(),
			}
			if opID != nil {
				out["fingerprint"] = opID.Fingerprint()
				out["hub_fingerprint"] = id.Fingerprint()
				out["name"] = opID.Name
			}
			return out
		},
		Invite: func() string { return blob },
		QR:     func() string { return blob },
		Banner: func() string { return banner },
		Become: func() (string, error) { return "already a hub", nil },
		Stop:   func() { srv.Stop(); stop() },
		Send: func(to, text string) error {
			if op == nil {
				return fmt.Errorf("no operator spoke; restart with default --operator")
			}
			chat.Add("me", text, true)
			return op.SendToName(to, text)
		},
		AddInvite: func(raw string) error {
			if op == nil {
				return fmt.Errorf("no operator spoke on this hub")
			}
			return op.AddInvite(raw)
		},
		Messages: chat.Snapshot,
		Peers: func() []map[string]any {
			if op == nil {
				return nil
			}
			return op.PeerRows()
		},
		Probe: func() map[string]any { return hub.ProbeSnapshot(port, dev) },
	})
	fmt.Println("role:", role, " panel:", o.api, " (Ctrl-C to stop)")
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
	home := homeFlag(fs)
	api := fs.String("api", envOr("STARMESH_API", "127.0.0.1:7780"), "web panel + JSON API (empty to disable)")
	token := fs.String("api-token", os.Getenv("STARMESH_API_TOKEN"), "optional bearer token if --api is not loopback")
	headless := fs.Bool("headless", false, "never read stdin (implied when stdin is not a TTY)")
	interactive := fs.Bool("interactive", false, "force the stdin chat prompt even without a TTY")
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
	chat := &hub.ChatLog{}
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
			who := n
			if who == "" {
				who = from
			}
			chat.Add(who, text, false)
			fmt.Printf("\r<%s %s> %s\n> ", n, from, text)
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
				_ = sp.AddInvite(blob)
				fmt.Fprintln(os.Stderr, "lan invite:", blob[:min(40, len(blob))], "...")
			}
		}()
	}
	go func() { _ = sp.Run(ctx) }()

	hub.ServeLocalAPI(*api, &hub.LocalAPI{
		Token:   *token,
		Started: time.Now(),
		Status: func() map[string]any {
			b := banner
			if b == "" {
				b = "No hub — queued"
			}
			return map[string]any{
				"role":        "spoke",
				"name":        *name,
				"banner":      b,
				"hubs":        sp.StatusHubs(),
				"fingerprint": id.Fingerprint(),
			}
		},
		Banner: func() string {
			if banner == "" {
				return "No hub — queued"
			}
			return banner
		},
		Messages: chat.Snapshot,
		Peers:    sp.PeerRows,
		Probe:    func() map[string]any { return hub.ProbeSnapshot(int(*port), false) },
		Send: func(to, text string) error {
			chat.Add("me", text, true)
			return sp.SendToName(to, text)
		},
		AddInvite: func(raw string) error {
			return sp.AddInvite(raw)
		},
		Become: func() (string, error) {
			c := hub.Probe(hub.ProbeOpts{Port: int(*port)})
			if !c.CanBeHub {
				msg := c.Reason
				if msg == "" {
					msg = hub.HubRefuse
				}
				return msg, fmt.Errorf("become-hub refused")
			}
			return "This host can be a hub. Stop this spoke and run: starmesh node --api 0.0.0.0:7780", nil
		},
	})

	fmt.Fprintf(os.Stderr, "spoke %s fp=%s\n", *name, id.Fingerprint())
	usePrompt := *interactive || (!*headless && hub.StdinIsTTY())
	if !usePrompt {
		slog.Info("headless spoke (no stdin); waiting for signals")
		<-ctx.Done()
		return nil
	}
	fmt.Fprintln(os.Stderr, "type messages; /peers  /to NAME  /status  /invite BLOB")
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
			if err := sp.AddInvite(strings.TrimSpace(strings.TrimPrefix(line, "/invite "))); err != nil {
				fmt.Fprintln(os.Stderr, err)
			} else {
				fmt.Println("invite queued for next dial")
			}
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
	return in.Err()
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

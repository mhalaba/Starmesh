package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/mhalaba/Starmesh/invite"
)

var ErrNoHub = errors.New("no hub — queued")

type SpokeConfig struct {
	Name       string
	Invites    []string
	ManualIPv6 string
	ManualPort uint16
	ManualKey  []byte
	Community  []CachedHub
	Seeds      []CachedHub
	Cache      *Cache
	Queue      *Queue
	Capture    bool
	Logger     *slog.Logger
	OnMessage  func(from string, name string, text string)
	OnStatus   func(banner string)
}

type Spoke struct {
	cfg    SpokeConfig
	id     *Identity
	log    *slog.Logger
	mu     sync.Mutex
	sess   *session
	hubOK  *HelloOK
	peers  map[string]peerInfo // ed hex -> x25519 + name
	cancel context.CancelFunc
	dump   []byte
	failed map[string]time.Time
}

type peerInfo struct {
	Ed   []byte
	X    [32]byte
	Name string
}

func NewSpoke(id *Identity, cfg SpokeConfig) *Spoke {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Name != "" {
		id.Name = cfg.Name
	}
	if cfg.Cache == nil {
		cfg.Cache = OpenCache("")
	}
	if cfg.Queue == nil {
		cfg.Queue = OpenQueue("", 1000)
	}
	return &Spoke{
		cfg:    cfg,
		id:     id,
		log:    cfg.Logger,
		peers:  map[string]peerInfo{},
		failed: map[string]time.Time{},
	}
}

func (sp *Spoke) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	sp.cancel = cancel
	defer cancel()
	var backoff time.Duration
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err := sp.connectOnce(ctx)
		if err == nil {
			backoff = 0
			continue
		}
		sp.banner(ErrNoHub.Error())
		if backoff == 0 {
			backoff = time.Second
		} else {
			backoff *= 2
			if backoff > 20*time.Second {
				backoff = 20 * time.Second
			}
		}
		sp.log.Info("hub unreachable, retrying", "err", err, "wait", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func (sp *Spoke) Stop() {
	if sp.cancel != nil {
		sp.cancel()
	}
	sp.mu.Lock()
	if sp.sess != nil {
		_ = sp.sess.c.Close()
	}
	sp.mu.Unlock()
}

func (sp *Spoke) connectOnce(ctx context.Context) error {
	cands := sp.candidates()
	if len(cands) == 0 {
		return errors.New("no hub candidates (scan an invite, paste IPv6, or provision community hubs)")
	}
	var last error
	var dropped []byte
	sp.mu.Lock()
	if sp.sess != nil {
		dropped = append([]byte(nil), sp.sess.ed...)
	}
	sp.mu.Unlock()
	for _, h := range cands {
		timeout := DialTimeout
		if sp.recentFail(h) || (len(dropped) > 0 && edEq(dropped, h.Ed25519)) {
			timeout = 2 * time.Second
		}
		dctx, cancel := context.WithTimeout(ctx, timeout)
		c, rtt, err := dialHub(dctx, h, sp.cfg.Capture)
		cancel()
		if err != nil {
			sp.noteFail(h)
			last = err
			continue
		}
		ok, err := sp.handshake(c, h)
		if err != nil {
			_ = c.Close()
			last = err
			continue
		}
		h.LastRTTMs = rtt.Milliseconds()
		h.Ed25519 = hexBytes(ok.Ed25519)
		h.X25519 = hexBytes(ok.X25519)
		h.Name = ok.Name
		h.CloudSeed = ok.CloudSeed
		if h.Source == "" {
			h.Source = SrcCache
		}
		sp.cfg.Cache.Remember(h)
		_ = sp.cfg.Cache.Save()
		sp.clearFail(h)

		sess := &session{c: c, role: RoleHub, ed: ok.Ed25519, x: ok.X25519, name: ok.Name, cloud: ok.CloudSeed, rtt: rtt, last: time.Now()}
		sp.mu.Lock()
		sp.sess = sess
		sp.hubOK = ok
		sp.mu.Unlock()

		kind := "IPv6 Starlink"
		if ok.CloudSeed {
			kind = "cloud seed"
		} else if h.IPv6 == "" && h.IPv4 != "" {
			kind = "IPv4"
		}
		sp.banner(fmt.Sprintf("Hub: %s (%s)", displayName(ok.Name, h), kind))
		sp.log.Info("spoke connected", "hub", ok.Name, "fp", Fingerprint(ok.Ed25519), "proto", c.Protocol(), "rtt", rtt)

		_ = sess.send(msgOf(TypePresence, Presence{PubKeys: [][]byte{sp.id.EdPub}, X25519: sp.id.XPub[:], Name: sp.id.Name}))
		for _, env := range sp.cfg.Queue.PopAll() {
			_ = sess.send(msgOf(TypeEnvelope, env))
		}
		err = sp.readLoop(ctx, sess)
		droppedHub := CachedHub{Ed25519: hexBytes(sess.ed), IPv6: h.IPv6, IPv4: h.IPv4, Port: h.Port}
		sp.mu.Lock()
		sp.sess = nil
		sp.hubOK = nil
		sp.mu.Unlock()
		sp.noteFail(droppedHub)
		_ = c.Close()
		_ = c.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if last == nil {
		last = errors.New("all hub candidates failed")
	}
	return last
}

func (sp *Spoke) handshake(c Conn, h CachedHub) (*HelloOK, error) {
	hello := Hello{
		Role:    RoleSpoke,
		Ed25519: sp.id.EdPub,
		X25519:  sp.id.XPub[:],
		Name:    sp.id.Name,
		Ts:      nowUnix(),
	}
	hello.Sign(sp.id.EdPriv)
	if err := c.WriteMsg(msgOf(TypeHello, hello)); err != nil {
		return nil, err
	}
	_ = setReadDeadline(c, HandshakeTimeout)
	m, err := c.ReadMsg()
	if err != nil {
		return nil, err
	}
	if m.Type != TypeHelloOK && m.Type != TypePeerOK {
		return nil, fmt.Errorf("unexpected %s", m.Type)
	}
	var ok HelloOK
	if err := json.Unmarshal(m.Body, &ok); err != nil {
		return nil, err
	}
	if err := ok.Verify(); err != nil {
		return nil, err
	}
	if len(h.Ed25519) == 32 && !edEq(ok.Ed25519, h.Ed25519) {
		return nil, errors.New("hub key mismatch")
	}
	return &ok, nil
}

func (sp *Spoke) readLoop(ctx context.Context, sess *session) error {
	go func() {
		t := time.NewTicker(KeepAlive)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = sess.send(msgOf(TypePing, Ping{T: time.Now().UnixNano()}))
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = setReadDeadline(sess.c, IdleTimeout)
		m, err := sess.c.ReadMsg()
		if err != nil {
			return err
		}
		if sp.cfg.Capture {
			sp.mu.Lock()
			sp.dump = sess.c.Dump()
			sp.mu.Unlock()
		}
		switch m.Type {
		case TypePong:
			var p Ping
			_ = json.Unmarshal(m.Body, &p)
			if p.T > 0 {
				sess.rtt = time.Since(time.Unix(0, p.T))
			}
		case TypePing:
			var p Ping
			_ = json.Unmarshal(m.Body, &p)
			_ = sess.send(msgOf(TypePong, Ping{T: p.T}))
		case TypePresenceGossip:
			var g PresenceGossip
			_ = json.Unmarshal(m.Body, &g)
			sp.mu.Lock()
			// A gossip without a Bloom filter is a full roster snapshot
			// (the hub only compresses at >=64 entries). Treat it as
			// authoritative so peers that have left the hub are pruned
			// instead of lingering forever (which would let SendToName
			// route to a dead session). Preserve any X25519 we already
			// learned directly from a peer's chat envelope.
			if g.Bloom == nil {
				fresh := make(map[string]peerInfo, len(g.Entries))
				for _, e := range g.Entries {
					if edEq(e.Pub, sp.id.EdPub) {
						continue
					}
					var x [32]byte
					if len(e.X25519) == 32 {
						copy(x[:], e.X25519)
					}
					k := mustHex(e.Pub)
					if x == [32]byte{} {
						if prev, ok := sp.peers[k]; ok && prev.X != [32]byte{} {
							x = prev.X
						}
					}
					fresh[k] = peerInfo{Ed: e.Pub, X: x, Name: e.Name}
				}
				sp.peers = fresh
			} else {
				for _, e := range g.Entries {
					if edEq(e.Pub, sp.id.EdPub) {
						continue
					}
					var x [32]byte
					if len(e.X25519) == 32 {
						copy(x[:], e.X25519)
					}
					sp.peers[mustHex(e.Pub)] = peerInfo{Ed: e.Pub, X: x, Name: e.Name}
				}
			}
			sp.mu.Unlock()
		case TypeEnvelope:
			var env Envelope
			if err := json.Unmarshal(m.Body, &env); err != nil {
				continue
			}
			sp.handleEnvelope(&env)
		}
	}
}

func (sp *Spoke) handleEnvelope(env *Envelope) {
	if err := env.Verify(); err != nil {
		return
	}
	if !edEq(env.To, sp.id.EdPub) {
		return
	}
	sp.mu.Lock()
	info, ok := sp.peers[mustHex(env.From)]
	sp.mu.Unlock()
	var fromX [32]byte
	if ok {
		fromX = info.X
	}
	// First-contact: ciphertext still opens if we learned X25519 via a
	// previous message header. Chat plaintext includes the sender X pub.
	plain, err := sp.tryOpen(env, fromX)
	if err != nil {
		sp.log.Debug("open failed", "err", err)
		return
	}
	var box chatBox
	if err := json.Unmarshal(plain, &box); err != nil {
		return
	}
	if len(box.X25519) == 32 {
		copy(fromX[:], box.X25519)
		sp.mu.Lock()
		sp.peers[mustHex(env.From)] = peerInfo{Ed: env.From, X: fromX, Name: box.Name}
		sp.mu.Unlock()
	}
	if sp.cfg.OnMessage != nil {
		sp.cfg.OnMessage(Fingerprint(env.From), box.Name, box.Text)
	}
}

type chatBox struct {
	Text   string `json:"text"`
	Name   string `json:"name,omitempty"`
	X25519 []byte `json:"x25519,omitempty"`
}

func (sp *Spoke) tryOpen(env *Envelope, fromX [32]byte) ([]byte, error) {
	if fromX != [32]byte{} {
		return Open(sp.id, fromX, env)
	}
	return nil, errors.New("unknown sender box key")
}

func (sp *Spoke) Send(toEd []byte, toX [32]byte, text string) error {
	plain, _ := json.Marshal(chatBox{Text: text, Name: sp.id.Name, X25519: sp.id.XPub[:]})
	env, err := Seal(sp.id, toX, toEd, EnvChat, plain)
	if err != nil {
		return err
	}
	sp.mu.Lock()
	sess := sp.sess
	sp.mu.Unlock()
	if sess == nil {
		sp.cfg.Queue.Push(env)
		sp.banner(ErrNoHub.Error())
		return ErrNoHub
	}
	if err := sess.send(msgOf(TypeEnvelope, env)); err != nil {
		sp.cfg.Queue.Push(env)
		_ = sess.c.Close()
		return err
	}
	return nil
}

func (sp *Spoke) SendToName(name, text string) error {
	sp.mu.Lock()
	var ed []byte
	var x [32]byte
	for _, p := range sp.peers {
		if p.Name == name || Fingerprint(p.Ed) == name {
			ed, x = p.Ed, p.X
			break
		}
	}
	// If only one other peer, use them.
	if ed == nil && len(sp.peers) == 1 {
		for _, p := range sp.peers {
			ed, x = p.Ed, p.X
		}
	}
	sp.mu.Unlock()
	if ed == nil {
		return fmt.Errorf("unknown peer %q (wait for presence or pass --to hex)", name)
	}
	if x == [32]byte{} {
		return fmt.Errorf("no X25519 for %s yet", name)
	}
	return sp.Send(ed, x, text)
}

func (sp *Spoke) Peers() []peerInfo {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	out := make([]peerInfo, 0, len(sp.peers))
	for _, p := range sp.peers {
		out = append(out, p)
	}
	return out
}

func (sp *Spoke) ConnectedHub() *HelloOK {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.hubOK
}

func (sp *Spoke) Dump() []byte {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return append([]byte(nil), sp.dump...)
}

func (sp *Spoke) banner(s string) {
	if sp.cfg.OnStatus != nil {
		sp.cfg.OnStatus(s)
	}
}

func (sp *Spoke) AddInvite(s string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.cfg.Invites = append(sp.cfg.Invites, s)
}

func (sp *Spoke) candidates() []CachedHub {
	var open, cached, community, fresh, seeds []CachedHub
	sp.mu.Lock()
	if sp.sess != nil && sp.hubOK != nil {
		h := CachedHub{
			Name: sp.hubOK.Name, Ed25519: hexBytes(sp.hubOK.Ed25519),
			Source: SrcOpen,
		}
		open = append(open, h)
	}
	invites := append([]string(nil), sp.cfg.Invites...)
	manualV6 := sp.cfg.ManualIPv6
	manualPort := sp.cfg.ManualPort
	manualKey := sp.cfg.ManualKey
	sp.mu.Unlock()
	if sp.cfg.Cache != nil {
		cached = sp.cfg.Cache.List()
	}
	community = sp.cfg.Community
	for _, raw := range invites {
		inv, err := invite.Decode(raw, combineResolvers(sp.cfg.Cache, community))
		if err != nil {
			sp.log.Warn("invite", "err", err)
			continue
		}
		h := cachedFromInvite(inv, SrcInvite)
		if sp.cfg.Cache != nil {
			sp.cfg.Cache.Remember(h)
		}
		if inv.CloudSeed {
			seeds = append(seeds, h)
		} else {
			fresh = append(fresh, h)
		}
	}
	if manualV6 != "" {
		h := CachedHub{IPv6: manualV6, Port: manualPort, Source: SrcInvite, Ed25519: hexBytes(manualKey)}
		if h.Port == 0 {
			h.Port = invite.DefaultPort
		}
		fresh = append(fresh, h)
	}
	seeds = append(seeds, sp.cfg.Seeds...)
	ordered := DialOrder(open, cached, community, fresh, seeds)
	var good, bad []CachedHub
	for _, h := range ordered {
		if sp.recentFail(h) {
			bad = append(bad, h)
		} else {
			good = append(good, h)
		}
	}
	return append(good, bad...)
}

func hubFailKey(h CachedHub) string {
	return string(h.Ed25519) + "|" + h.IPv6 + "|" + h.IPv4 + "|" + fmt.Sprintf("%d", h.Port)
}

func (sp *Spoke) noteFail(h CachedHub) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.failed == nil {
		sp.failed = map[string]time.Time{}
	}
	sp.failed[hubFailKey(h)] = time.Now()
}

func (sp *Spoke) clearFail(h CachedHub) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	delete(sp.failed, hubFailKey(h))
}

func (sp *Spoke) recentFail(h CachedHub) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	t, ok := sp.failed[hubFailKey(h)]
	if !ok {
		return false
	}
	return time.Since(t) < 30*time.Second
}

func cachedFromInvite(inv *invite.Invite, src HubSource) CachedHub {
	h := CachedHub{
		Name: inv.Name, Port: inv.Port, Source: src, CloudSeed: inv.CloudSeed,
		Ed25519: hexBytes(inv.PubKey[:]), Short: inv.ShortDisplay(),
	}
	if inv.IPv6 != nil {
		h.IPv6 = inv.IPv6.String()
	}
	if inv.IPv4 != nil {
		h.IPv4 = inv.IPv4.String()
	}
	return h
}

func combineResolvers(cache *Cache, community []CachedHub) invite.ShortResolver {
	var invs []*invite.Invite
	if cache != nil {
		for _, h := range cache.List() {
			if inv := h.Invite(); inv != nil {
				invs = append(invs, inv)
			}
		}
	}
	for _, h := range community {
		if inv := h.Invite(); inv != nil {
			invs = append(invs, inv)
		}
	}
	return invite.LookupTable(invs)
}

func dialHub(ctx context.Context, h CachedHub, capture bool) (Conn, time.Duration, error) {
	var addrs []struct{ netw, addr string }
	if h.IPv6 != "" {
		port := h.Port
		if port == 0 {
			port = invite.DefaultPort
		}
		a := net.JoinHostPort(h.IPv6, fmt.Sprintf("%d", port))
		addrs = append(addrs, struct{ netw, addr string }{"quic", a})
		addrs = append(addrs, struct{ netw, addr string }{"tcp6", a})
	}
	if h.IPv4 != "" {
		port := h.Port
		if port == 0 {
			port = invite.DefaultPort
		}
		a := net.JoinHostPort(h.IPv4, fmt.Sprintf("%d", port))
		addrs = append(addrs, struct{ netw, addr string }{"quic", a})
		addrs = append(addrs, struct{ netw, addr string }{"tcp4", a})
	}
	var last error
	for _, a := range addrs {
		start := time.Now()
		c, err := Dial(ctx, a.netw, a.addr, capture)
		if err != nil {
			last = err
			continue
		}
		return c, time.Since(start), nil
	}
	if last == nil {
		last = errors.New("no addresses")
	}
	return nil, 0, last
}

func displayName(name string, h CachedHub) string {
	if name != "" {
		return name
	}
	if h.Name != "" {
		return h.Name
	}
	return Fingerprint(h.Ed25519)
}

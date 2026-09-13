package hub

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LocalAPI is the control plane for the operator web panel and Flutter UI.
// Binding 127.0.0.1 keeps it off the LAN. Binding 0.0.0.0:PORT exposes the
// panel to phones; pair that with Token. It is not a public mesh relay.
type LocalAPI struct {
	mu        sync.Mutex
	Token     string
	Started   time.Time
	Status    func() map[string]any
	Invite    func() string
	QR        func() string
	Become    func() (string, error)
	Stop      func()
	Send      func(to, text string) error
	Banner    func() string
	AddInvite func(raw string) error
	Messages  func() []map[string]any
	Peers     func() []map[string]any
	Probe     func() map[string]any
}

// ServeLocalAPI starts the operator panel + JSON API on addr.
// Empty addr disables the listener.
func ServeLocalAPI(addr string, api *LocalAPI) {
	if addr == "" || api == nil {
		return
	}
	if api.Started.IsZero() {
		api.Started = time.Now()
	}
	go func() {
		if !APIIsLoopback(addr) && api.Token == "" {
			slog.Warn("local api is reachable on the LAN without --api-token")
		}
		slog.Info("local api", "addr", addr, "ui", true, "token", api.Token != "")
		if err := http.ListenAndServe(addr, api.Handler()); err != nil {
			slog.Error("local api", "err", err)
		}
	}()
}

func (a *LocalAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", a.handleStatus)
	mux.HandleFunc("/v1/invite", a.handleInvite)
	mux.HandleFunc("/v1/become-hub", a.handleBecome)
	mux.HandleFunc("/v1/stop-hub", a.handleStop)
	mux.HandleFunc("/v1/send", a.handleSend)
	mux.HandleFunc("/v1/peers", a.handlePeers)
	mux.HandleFunc("/v1/probe", a.handleProbe)
	mux.HandleFunc("/v1/qr.svg", a.handleQR)
	mux.Handle("/", http.FileServer(webFileSystem()))
	return withCORS(withToken(a.Token, mux))
}

func (a *LocalAPI) snapshotStatus() map[string]any {
	a.mu.Lock()
	fn := a.Status
	ban := a.Banner
	msgs := a.Messages
	peers := a.Peers
	probe := a.Probe
	inv := a.Invite
	started := a.Started
	a.mu.Unlock()
	out := map[string]any{}
	if fn != nil {
		out = fn()
	}
	if ban != nil {
		out["banner"] = ban()
	}
	if msgs != nil {
		out["messages"] = msgs()
	}
	if peers != nil {
		out["peers"] = peers()
	}
	if probe != nil {
		out["probe"] = probe()
	}
	if inv != nil {
		if _, ok := out["invite"]; !ok {
			out["invite"] = inv()
		}
	}
	if started.IsZero() {
		started = time.Now()
	}
	out["uptime_s"] = int(time.Since(started).Seconds())
	if _, ok := out["chat_e2e"]; !ok {
		out["chat_e2e"] = true
		out["chat_box"] = "nacl-x25519"
	}
	return out
}

func (a *LocalAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.snapshotStatus())
}

func (a *LocalAPI) handleInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			Invite string `json:"invite"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.mu.Lock()
		fn := a.AddInvite
		a.mu.Unlock()
		if fn == nil {
			http.Error(w, "unavailable", 503)
			return
		}
		if err := fn(req.Invite); err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"ok": "1"})
		return
	}
	a.mu.Lock()
	fn := a.Invite
	qr := a.QR
	a.mu.Unlock()
	out := map[string]string{}
	if fn != nil {
		out["invite"] = fn()
	}
	if qr != nil {
		out["qr_payload"] = qr()
	}
	writeJSON(w, out)
}

func (a *LocalAPI) handleBecome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", 405)
		return
	}
	a.mu.Lock()
	fn := a.Become
	a.mu.Unlock()
	if fn == nil {
		http.Error(w, "unavailable", 503)
		return
	}
	msg, err := fn()
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error(), "message": msg})
		return
	}
	writeJSON(w, map[string]string{"ok": "1", "message": msg})
}

func (a *LocalAPI) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", 405)
		return
	}
	a.mu.Lock()
	fn := a.Stop
	a.mu.Unlock()
	if fn != nil {
		fn()
	}
	writeJSON(w, map[string]string{"ok": "1"})
}

func (a *LocalAPI) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST", 405)
		return
	}
	var req struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.mu.Lock()
	fn := a.Send
	a.mu.Unlock()
	if fn == nil {
		http.Error(w, "unavailable", 503)
		return
	}
	if err := fn(req.To, req.Text); err != nil {
		writeJSON(w, map[string]any{"error": err.Error(), "unknown_peer": strings.Contains(err.Error(), "unknown peer")})
		return
	}
	writeJSON(w, map[string]string{"ok": "1"})
}

func (a *LocalAPI) handlePeers(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	fn := a.Peers
	a.mu.Unlock()
	out := []map[string]any{}
	if fn != nil {
		if p := fn(); p != nil {
			out = p
		}
	}
	writeJSON(w, map[string]any{"peers": out})
}

func (a *LocalAPI) handleProbe(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	fn := a.Probe
	a.mu.Unlock()
	out := map[string]any{}
	if fn != nil {
		out = fn()
	}
	writeJSON(w, out)
}

func (a *LocalAPI) handleQR(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	fn := a.QR
	if fn == nil {
		fn = a.Invite
	}
	a.mu.Unlock()
	payload := ""
	if fn != nil {
		payload = fn()
	}
	svg, err := qrSVG(payload)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(svg))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Starmesh-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}


// ProbeSnapshot is the Network tab payload (capability + RA).
func ProbeSnapshot(port int, dev bool) map[string]any {
	c := Probe(ProbeOpts{Port: port, Dev: dev})
	ra := ProbeRA()
	return map[string]any{
		"can_be_hub":    c.CanBeHub,
		"reason":        c.Reason,
		"dev":           dev || c.DevRelaxed,
		"global_ipv6":   ipList(c.GlobalIPv6),
		"has_ipv6":      c.HasDefaultIPv6 && !devOnlyIPv6(c),
		"lab_ipv6":      dev || c.DevRelaxed,
		"can_bind_udp":  c.CanBindUDP,
		"bind_port":     c.BindPort,
		"bind_error":    c.BindError,
		"behind_cgnat":  c.BehindCGNAT,
		"claim_ipv4":    c.ClaimIPv4,
		"public_ipv4":   ipString(c.PublicIPv4),
		"lab_ipv4":      ipString(PickLabIPv4()),
		"ra":            ra,
		"refuse":        HubRefuse,
	}
}

func ipList(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func devOnlyIPv6(c Cap) bool {
	if !c.DevRelaxed {
		return false
	}
	for _, ip := range c.GlobalIPv6 {
		if isGlobalIPv6(ip) {
			return false
		}
	}
	return true
}

package hub

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
)

// LocalAPI is a loopback control plane for the Flutter app / operator UI.
// It is not a public relay and must stay on 127.0.0.1.
// Flutter talks here only — seed/hub QUIC dialing stays in this Go process.
type LocalAPI struct {
	mu        sync.Mutex
	Status    func() map[string]any
	Invite    func() string
	QR        func() string
	Become    func() (string, error)
	Stop      func()
	Send      func(to, text string) error
	Banner    func() string
	AddInvite func(raw string) error
	Messages  func() []map[string]any
}

// ServeLocalAPI starts the Flutter control plane on addr (typically 127.0.0.1:7780).
// Empty addr disables the listener.
func ServeLocalAPI(addr string, api *LocalAPI) {
	if addr == "" || api == nil {
		return
	}
	go func() {
		slog.Info("local api", "addr", addr)
		if err := http.ListenAndServe(addr, api.Handler()); err != nil {
			slog.Error("local api", "err", err)
		}
	}()
}

func (a *LocalAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		fn := a.Status
		ban := a.Banner
		msgs := a.Messages
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
		writeJSON(w, out)
	})
	mux.HandleFunc("/v1/invite", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/v1/become-hub", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/v1/stop-hub", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/v1/send", func(w http.ResponseWriter, r *http.Request) {
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
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]string{"ok": "1"})
	})
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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

// UIChatLine is a chat row returned to the Flutter UI via /v1/status.
type UIChatLine struct {
	From string `json:"from"`
	Text string `json:"text"`
	Mine bool   `json:"mine"`
}

// ChatLog is a small in-memory transcript for the local UI.
type ChatLog struct {
	mu    sync.Mutex
	lines []UIChatLine
}

func (l *ChatLog) Add(from, text string, mine bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, UIChatLine{From: from, Text: text, Mine: mine})
	if len(l.lines) > 500 {
		l.lines = l.lines[len(l.lines)-400:]
	}
}

func (l *ChatLog) Snapshot() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]map[string]any, 0, len(l.lines))
	for _, line := range l.lines {
		out = append(out, map[string]any{
			"from": line.From,
			"text": line.Text,
			"mine": line.Mine,
		})
	}
	return out
}

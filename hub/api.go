package hub

import (
	"encoding/json"
	"net/http"
	"sync"
)

// LocalAPI is a loopback control plane for the Flutter app / operator UI.
// It is not a public relay and must stay on 127.0.0.1.
type LocalAPI struct {
	mu     sync.Mutex
	Status func() map[string]any
	Invite func() string
	QR     func() string
	Become func() (string, error)
	Stop   func()
	Send   func(to, text string) error
	Banner func() string
}

func (a *LocalAPI) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		fn := a.Status
		ban := a.Banner
		a.mu.Unlock()
		out := map[string]any{}
		if fn != nil {
			out = fn()
		}
		if ban != nil {
			out["banner"] = ban()
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("/v1/invite", func(w http.ResponseWriter, r *http.Request) {
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
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

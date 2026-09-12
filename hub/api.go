package hub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// LocalAPI is a loopback control plane for the Flutter app / operator UI.
// It is not a public relay and must stay on 127.0.0.1. The same type backs
// both a hub process (`hub --api`) and a spoke process (`spoke --api`), so
// the Flutter app can drive a phone (spoke) or a station (hub) identically.
type LocalAPI struct {
	mu        sync.Mutex
	Status    func() map[string]any
	Invite    func() string
	QR        func() string
	Become    func() (string, error)
	Stop      func()
	Send      func(to, text string) error
	AddInvite func(blob string)
	Banner    func() string

	msgs []APIMessage
	seq  int
}

// APIMessage is one chat line exposed to the local UI via /v1/messages.
type APIMessage struct {
	Seq  int    `json:"seq"`
	From string `json:"from"`
	Name string `json:"name"`
	Text string `json:"text"`
	Mine bool   `json:"mine"`
	Ts   int64  `json:"ts"`
}

const maxInbox = 1000

// Push records a chat message into the local inbox for the UI to poll.
func (a *LocalAPI) Push(from, name, text string, mine bool) {
	a.mu.Lock()
	a.seq++
	a.msgs = append(a.msgs, APIMessage{
		Seq: a.seq, From: from, Name: name, Text: text, Mine: mine,
		Ts: time.Now().UnixMilli(),
	})
	if len(a.msgs) > maxInbox {
		a.msgs = a.msgs[len(a.msgs)-maxInbox:]
	}
	a.mu.Unlock()
}

func (a *LocalAPI) messagesAfter(after int) ([]APIMessage, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]APIMessage, 0, len(a.msgs))
	for _, m := range a.msgs {
		if m.Seq > after {
			out = append(out, m)
		}
	}
	return out, a.seq
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
		if r.Method == http.MethodPost {
			var req struct {
				Invite string `json:"invite"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			a.mu.Lock()
			add := a.AddInvite
			a.mu.Unlock()
			if add == nil {
				http.Error(w, "unavailable", 503)
				return
			}
			add(req.Invite)
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
			a.Push("me", "me", req.Text, true)
			writeJSON(w, map[string]string{"error": err.Error(), "queued": "1"})
			return
		}
		a.Push("me", "me", req.Text, true)
		writeJSON(w, map[string]string{"ok": "1"})
	})
	// Long-poll inbox: returns as soon as a message with Seq > after exists,
	// or after a bounded wait so clients can re-poll cheaply.
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		deadline := time.Now().Add(25 * time.Second)
		for {
			msgs, latest := a.messagesAfter(after)
			if len(msgs) > 0 || time.Now().After(deadline) {
				writeJSON(w, map[string]any{"messages": msgs, "seq": latest})
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
		}
	})
	return withCORS(mux)
}

// withCORS allows the Flutter web build (served from another loopback port)
// to reach this loopback-only control plane during development.
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

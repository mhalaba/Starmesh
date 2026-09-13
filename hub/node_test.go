package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIIsLoopback(t *testing.T) {
	if !APIIsLoopback("127.0.0.1:7780") || !APIIsLoopback("[::1]:7780") {
		t.Fatal("loopback")
	}
	if APIIsLoopback("0.0.0.0:7780") || APIIsLoopback(":7780") {
		t.Fatal("LAN bind must not look loopback")
	}
}

func TestHeadlessAPIStaysUp(t *testing.T) {
	// Simulates systemd: the CLI "returns" from the stdin loop; the API
	// handler must keep serving until the process is signalled.
	chat := &ChatLog{}
	api := &LocalAPI{
		Started: time.Now(),
		Status: func() map[string]any {
			return map[string]any{"role": "spoke", "banner": "No hub — queued"}
		},
		Messages: chat.Snapshot,
		Send: func(to, text string) error {
			chat.Add("me", text, true)
			if to == "" {
				return nil
			}
			return nil
		},
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	// "main returned from stdin EOF"
	time.Sleep(20 * time.Millisecond)

	res, err := http.Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"role":"spoke"`) {
		t.Fatalf("body=%s", body)
	}
	res, err = http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("ui %d", res.StatusCode)
	}
}

func TestOperatorSendPathViaAPI(t *testing.T) {
	h := startDevHub(t, "tasai-lab")
	blob := inviteBlob(t, h)
	chat := &ChatLog{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	opHome := t.TempDir()
	op, opID, chat, err := StartOperatorSpoke(ctx, opHome, "tas", []string{blob}, chat)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.WaitUntilHubConnected(ctx, 8*time.Second); err != nil {
		t.Fatal(err)
	}
	bob, _, bobLog := startSpoke(t, "bob", []string{blob})
	waitHub(t, bob)
	_ = waitPeer(t, op, "bob")
	_ = waitPeer(t, bob, "tas")

	api := &LocalAPI{
		Send: func(to, text string) error {
			return chat.AfterSeal("me", text, true, op.SendToName(to, text))
		},
		Messages: chat.Snapshot,
		Peers:    op.PeerRows,
		Status: func() map[string]any {
			return map[string]any{"role": "node", "name": opID.Name, "fingerprint": opID.Fingerprint()}
		},
	}
	ts := httptest.NewServer(api.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Post(ts.URL+"/v1/send", "application/json", strings.NewReader(`{"to":"bob","text":"czesc-z-panelu"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != "1" {
		t.Fatalf("send=%v", out)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(bobLog.String(), "czesc-z-panelu") {
			st, err := http.Get(ts.URL + "/v1/status")
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.NewDecoder(st.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			st.Body.Close()
			if body["chat_e2e"] != true {
				t.Fatalf("chat_e2e=%v", body["chat_e2e"])
			}
			msgs, _ := body["messages"].([]any)
			if len(msgs) == 0 {
				t.Fatal("expected sealed UI line")
			}
			row := msgs[0].(map[string]any)
			if row["e2e"] != true || row["text"] != "czesc-z-panelu" {
				t.Fatalf("msg=%v", row)
			}
			for _, dump := range h.Dumps() {
				if strings.Contains(string(dump), "czesc-z-panelu") {
					t.Fatal("hub wire dump contained plaintext")
				}
			}
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("bob missed panel chat: %q", bobLog.String())
}

func TestAPITokenAndQR(t *testing.T) {
	api := &LocalAPI{
		Token:  "sekret",
		Invite: func() string { return "starmesh1:LAB" },
		QR:     func() string { return "starmesh1:LAB" },
		Status: func() map[string]any { return map[string]any{"role": "node"} },
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", res.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/qr.svg", nil)
	req.Header.Set("X-Starmesh-Token", "sekret")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(b), "<svg") {
		t.Fatalf("qr %d %s", res.StatusCode, b[:min(80, len(b))])
	}
}

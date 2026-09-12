package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalAPIStatusAndCORS(t *testing.T) {
	api := &LocalAPI{
		Status: func() map[string]any {
			return map[string]any{"role": "spoke", "banner": "No hub — queued"}
		},
		Banner: func() string { return "No hub — queued" },
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/v1/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("OPTIONS status %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("missing CORS")
	}

	res, err = http.Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["role"] != "spoke" {
		t.Fatalf("role=%v", got["role"])
	}
	if got["banner"] != "No hub — queued" {
		t.Fatalf("banner=%v", got["banner"])
	}
}

func TestLocalAPIIngestInvite(t *testing.T) {
	var got string
	api := &LocalAPI{
		AddInvite: func(raw string) error {
			got = raw
			return nil
		},
		Invite: func() string { return "hub-own-invite" },
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/v1/invite", "application/json", strings.NewReader(`{"invite":"starmesh1:TEST"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST status %d", res.StatusCode)
	}
	if got != "starmesh1:TEST" {
		t.Fatalf("AddInvite got %q", got)
	}

	res, err = http.Get(srv.URL + "/v1/invite")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]string
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["invite"] != "hub-own-invite" {
		t.Fatalf("GET invite=%v", out)
	}
}

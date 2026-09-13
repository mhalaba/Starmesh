package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// OperatorHome is the spoke identity used by a hub/node process for chat.
// It must not share identity.json with the hub, or fingerprints collide.
func OperatorHome(hubHome string) string {
	return filepath.Join(hubHome, "operator")
}

// StartOperatorSpoke dials the local hub invite and records chat for the UI.
func StartOperatorSpoke(ctx context.Context, home, name string, invites []string, chat *ChatLog) (*Spoke, *Identity, error) {
	id, err := LoadOrCreateIdentity(home, name)
	if err != nil {
		return nil, nil, err
	}
	if name != "" {
		id.Name = name
	}
	sp := NewSpoke(id, SpokeConfig{
		Name:    id.Name,
		Invites: invites,
		OnMessage: func(from, n, text string) {
			who := n
			if who == "" {
				who = from
			}
			chat.Add(who, text, false)
		},
		OnStatus: func(s string) {
			// Hub banner stays authoritative for a node process.
			_ = s
		},
	})
	go func() { _ = sp.Run(ctx) }()
	return sp, id, nil
}

func (sp *Spoke) PeerRows() []map[string]any {
	var out []map[string]any
	for _, p := range sp.Peers() {
		out = append(out, map[string]any{
			"name":        p.Name,
			"fingerprint": Fingerprint(p.Ed),
		})
	}
	return out
}

// WaitUntilHubConnected blocks until the spoke has a hub or ctx is done.
func (sp *Spoke) WaitUntilHubConnected(ctx context.Context, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sp.ConnectedHub() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(40 * time.Millisecond):
		}
	}
	return fmt.Errorf("operator spoke did not connect to local hub")
}


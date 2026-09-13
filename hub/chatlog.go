package hub

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/nacl/secretbox"
)

const chatLogMagic = "starmesh-chatlog-v1"

// UIChatLine is a chat row returned via /v1/status after local decrypt.
type UIChatLine struct {
	From string `json:"from"`
	Text string `json:"text"`
	Mine bool   `json:"mine"`
	E2E  bool   `json:"e2e"`
}

// ChatLog is the local UI transcript. The mesh already carries NaCl boxes;
// this file is a second box so a stolen --home does not dump plaintext chat.
type ChatLog struct {
	mu    sync.Mutex
	path  string
	key   [32]byte
	lines []UIChatLine
}

func chatLogKey(id *Identity) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(chatLogMagic))
	_, _ = h.Write(id.XPriv[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// OpenChatLog loads an encrypted transcript from home/chat.box.
// Empty home or nil identity keeps the log in memory only.
func OpenChatLog(home string, id *Identity) *ChatLog {
	l := &ChatLog{}
	if home == "" || id == nil {
		return l
	}
	l.path = filepath.Join(home, "chat.box")
	l.key = chatLogKey(id)
	l.load()
	return l
}

// AfterSeal records a line only if Seal ran (sent or queued). Unknown
// peer / missing X25519 does not write plaintext into the UI log.
func (l *ChatLog) AfterSeal(from, text string, mine bool, err error) error {
	if err != nil && !errors.Is(err, ErrNoHub) {
		return err
	}
	l.Add(from, text, mine)
	return err
}

func (l *ChatLog) Add(from, text string, mine bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, UIChatLine{From: from, Text: text, Mine: mine, E2E: true})
	if len(l.lines) > 500 {
		l.lines = l.lines[len(l.lines)-400:]
	}
	l.saveLocked()
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
			"e2e":  line.E2E,
		})
	}
	return out
}

func (l *ChatLog) saveLocked() {
	if l.path == "" {
		return
	}
	raw, err := json.Marshal(l.lines)
	if err != nil {
		return
	}
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return
	}
	out := secretbox.Seal(nonce[:], raw, &nonce, &l.key)
	_ = os.MkdirAll(filepath.Dir(l.path), 0o700)
	_ = os.WriteFile(l.path, out, 0o600)
}

func (l *ChatLog) load() {
	if l.path == "" {
		return
	}
	b, err := os.ReadFile(l.path)
	if err != nil || len(b) < 24+secretbox.Overhead {
		return
	}
	var nonce [24]byte
	copy(nonce[:], b[:24])
	plain, ok := secretbox.Open(nil, b[24:], &nonce, &l.key)
	if !ok {
		return
	}
	var lines []UIChatLine
	if err := json.Unmarshal(plain, &lines); err != nil {
		return
	}
	l.lines = lines
}

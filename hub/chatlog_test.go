package hub

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestChatLogSecretboxHidesPlaintext(t *testing.T) {
	id, err := GenerateIdentity("tas")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := id.Save(filepath.Join(home, "identity.json")); err != nil {
		t.Fatal(err)
	}
	secret := "tajna-rozmowa-box"
	l := OpenChatLog(home, id)
	l.Add("me", secret, true)

	raw, err := os.ReadFile(filepath.Join(home, "chat.box"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("plaintext leaked into chat.box")
	}

	again := OpenChatLog(home, id)
	snap := again.Snapshot()
	if len(snap) != 1 || snap[0]["text"] != secret || snap[0]["e2e"] != true {
		t.Fatalf("reload=%v", snap)
	}

	other, _ := GenerateIdentity("thief")
	stolen := OpenChatLog(home, other)
	if len(stolen.Snapshot()) != 0 {
		t.Fatal("another identity must not open the chat box")
	}
}

func TestAfterSealSkipsFailedSend(t *testing.T) {
	l := &ChatLog{}
	err := l.AfterSeal("me", "nope", true, errUnknownPeer())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(l.Snapshot()) != 0 {
		t.Fatal("must not log unsealed text")
	}
	if err := l.AfterSeal("me", "queued", true, ErrNoHub); err != ErrNoHub {
		t.Fatalf("queued %v", err)
	}
	if len(l.Snapshot()) != 1 {
		t.Fatal("queued sealed line should stay in the UI")
	}
}

func errUnknownPeer() error {
	return errString("unknown peer \"ghost\"")
}

type errString string

func (e errString) Error() string { return string(e) }

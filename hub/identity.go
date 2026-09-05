package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"
)

// Identity is one install: Ed25519 for signatures, X25519 for boxes.
// Relays never get the private keys; they only forward envelopes.
type Identity struct {
	EdPriv ed25519.PrivateKey `json:"-"`
	EdPub  ed25519.PublicKey  `json:"ed25519"`
	XPriv  [32]byte           `json:"-"`
	XPub   [32]byte           `json:"x25519"`
	Name   string             `json:"name,omitempty"`
}

type identityFile struct {
	EdPriv hexBytes `json:"ed25519_private"`
	EdPub  hexBytes `json:"ed25519_public"`
	XPriv  hexBytes `json:"x25519_private"`
	XPub   hexBytes `json:"x25519_public"`
	Name   string   `json:"name,omitempty"`
}

func GenerateIdentity(name string) (*Identity, error) {
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	var xPriv [32]byte
	if _, err := rand.Read(xPriv[:]); err != nil {
		return nil, err
	}
	// RFC 7748 clamp.
	xPriv[0] &= 248
	xPriv[31] &= 127
	xPriv[31] |= 64
	var xPub [32]byte
	curve25519.ScalarBaseMult(&xPub, &xPriv)
	return &Identity{
		EdPriv: edPriv,
		EdPub:  edPub,
		XPriv:  xPriv,
		XPub:   xPub,
		Name:   name,
	}, nil
}

func LoadIdentity(path string) (*Identity, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f identityFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	id := &Identity{
		EdPriv: ed25519.PrivateKey(f.EdPriv),
		EdPub:  ed25519.PublicKey(f.EdPub),
		Name:   f.Name,
	}
	copy(id.XPriv[:], f.XPriv)
	copy(id.XPub[:], f.XPub)
	if len(id.EdPriv) != ed25519.PrivateKeySize || len(id.EdPub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("identity: bad ed25519 size")
	}
	if len(f.XPriv) != 32 || len(f.XPub) != 32 {
		return nil, fmt.Errorf("identity: bad x25519 size")
	}
	return id, nil
}

func (id *Identity) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f := identityFile{
		EdPriv: hexBytes(id.EdPriv),
		EdPub:  hexBytes(id.EdPub),
		XPriv:  hexBytes(id.XPriv[:]),
		XPub:   hexBytes(id.XPub[:]),
		Name:   id.Name,
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (id *Identity) Fingerprint() string {
	return Fingerprint(id.EdPub)
}

func Fingerprint(pub []byte) string {
	if len(pub) == 0 {
		return ""
	}
	// Same Crockford 10-char scheme as invite short codes, over the pubkey.
	h := sha256Short(pub)
	return h
}

func (id *Identity) PubHex() string {
	return fmt.Sprintf("%x", []byte(id.EdPub))
}

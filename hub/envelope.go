package hub

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/nacl/box"
)

const (
	EnvChat       = "chat"
	EnvPresence   = "presence"
	nonceSize     = 24
	maxPlaintext  = 32 << 10
	maxCiphertext = 64 << 10
)

// Envelope is the only application payload that crosses a hub.
// Hubs see id/from/to/ts/type/sig/ciphertext and never the plaintext.
type Envelope struct {
	ID         string `json:"id"`
	From       []byte `json:"from"`
	To         []byte `json:"to"`
	Ts         int64  `json:"ts"`
	Type       string `json:"type"`
	Sig        []byte `json:"sig"`
	Ciphertext []byte `json:"ciphertext"`
}

func signedBytes(e *Envelope) []byte {
	// Canonical: length-prefixed fields, no JSON whitespace games.
	var out []byte
	put := func(b []byte) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(b)))
		out = append(out, n[:]...)
		out = append(out, b...)
	}
	put([]byte(e.ID))
	put(e.From)
	put(e.To)
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(e.Ts))
	put(ts[:])
	put([]byte(e.Type))
	put(e.Ciphertext)
	return out
}

func (e *Envelope) Sign(priv ed25519.PrivateKey) {
	e.Sig = ed25519.Sign(priv, signedBytes(e))
}

func (e *Envelope) Verify() error {
	if len(e.From) != ed25519.PublicKeySize {
		return errors.New("envelope: bad from")
	}
	if !ed25519.Verify(ed25519.PublicKey(e.From), signedBytes(e), e.Sig) {
		return errors.New("envelope: bad signature")
	}
	if e.Ts == 0 {
		return errors.New("envelope: missing ts")
	}
	if len(e.Ciphertext) < nonceSize+16 {
		return errors.New("envelope: ciphertext too small")
	}
	if len(e.Ciphertext) > maxCiphertext {
		return errors.New("envelope: ciphertext too large")
	}
	return nil
}

func Seal(from *Identity, toXPub [32]byte, toEd []byte, typ string, plaintext []byte) (*Envelope, error) {
	if len(plaintext) > maxPlaintext {
		return nil, errors.New("envelope: plaintext too large")
	}
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	sealed := box.Seal(nonce[:], plaintext, &nonce, &toXPub, &from.XPriv)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	env := &Envelope{
		ID:         fmt.Sprintf("%x", id),
		From:       append([]byte(nil), from.EdPub...),
		To:         append([]byte(nil), toEd...),
		Ts:         time.Now().Unix(),
		Type:       typ,
		Ciphertext: sealed,
	}
	env.Sign(from.EdPriv)
	return env, nil
}

func Open(to *Identity, fromXPub [32]byte, env *Envelope) ([]byte, error) {
	if err := env.Verify(); err != nil {
		return nil, err
	}
	if len(env.Ciphertext) < nonceSize {
		return nil, errors.New("envelope: short box")
	}
	var nonce [nonceSize]byte
	copy(nonce[:], env.Ciphertext[:nonceSize])
	plain, ok := box.Open(nil, env.Ciphertext[nonceSize:], &nonce, &fromXPub, &to.XPriv)
	if !ok {
		return nil, errors.New("envelope: open failed")
	}
	return plain, nil
}

type ChatPlain struct {
	Text string `json:"text"`
	Name string `json:"name,omitempty"`
}

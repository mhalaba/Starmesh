package hub

import (
	"bytes"
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	a, err := GenerateIdentity("alice")
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateIdentity("bob")
	if err != nil {
		t.Fatal(err)
	}
	env, err := Seal(a, b.XPub, b.EdPub, EnvChat, []byte(`{"text":"secret-hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Verify(); err != nil {
		t.Fatal(err)
	}
	plain, err := Open(b, a.XPub, env)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, []byte(`{"text":"secret-hello"}`)) {
		t.Fatalf("plain %s", plain)
	}
	c, _ := GenerateIdentity("carol")
	if _, err := Open(c, a.XPub, env); err == nil {
		t.Fatal("carol must not open a box sealed to bob")
	}
	if len(env.FromX) != 32 {
		t.Fatal("seal must put sender X25519 on the envelope")
	}
}

func TestOpenFirstContactFromX(t *testing.T) {
	a, _ := GenerateIdentity("alice")
	b, _ := GenerateIdentity("bob")
	env, err := Seal(a, b.XPub, b.EdPub, EnvChat, []byte("first-contact"))
	if err != nil {
		t.Fatal(err)
	}
	var hint [32]byte
	copy(hint[:], env.FromX)
	plain, err := Open(b, hint, env)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "first-contact" {
		t.Fatalf("plain %s", plain)
	}
}

func TestSpokeOpensWithoutPeerTable(t *testing.T) {
	a, _ := GenerateIdentity("alice")
	b, _ := GenerateIdentity("bob")
	env, err := Seal(a, b.XPub, b.EdPub, EnvChat, []byte(`{"text":"solo","name":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	sp := NewSpoke(b, SpokeConfig{Name: "bob"})
	plain, err := sp.tryOpen(env, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(plain, []byte("solo")) {
		t.Fatalf("plain %s", plain)
	}
}

func TestEnvelopeTamper(t *testing.T) {
	a, _ := GenerateIdentity("a")
	b, _ := GenerateIdentity("b")
	env, _ := Seal(a, b.XPub, b.EdPub, EnvChat, []byte("hi"))
	env.Ciphertext[20] ^= 0xff
	if env.Verify() == nil {
		t.Fatal("expected verify fail")
	}
}

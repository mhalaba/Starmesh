package hub

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	TypeHello          = "hello"
	TypeHelloOK        = "hello_ok"
	TypePeerHello      = "peer_hello"
	TypePeerOK         = "peer_ok"
	TypeEnvelope       = "envelope"
	TypePresence       = "presence"
	TypePresenceGossip = "presence_gossip"
	TypePing           = "ping"
	TypePong           = "pong"
	TypeError          = "error"

	MaxFrame = 1 << 20
	ALPN     = "starmesh/1"
)

type Role string

const (
	RoleSpoke Role = "spoke"
	RoleHub   Role = "hub"
)

type Message struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

type Hello struct {
	Role       Role   `json:"role"`
	Ed25519    []byte `json:"ed25519"`
	X25519     []byte `json:"x25519"`
	Name       string `json:"name,omitempty"`
	Ts         int64  `json:"ts"`
	CloudSeed  bool   `json:"cloud_seed,omitempty"`
	ListenIPv6 string `json:"listen_ipv6,omitempty"`
	ListenIPv4 string `json:"listen_ipv4,omitempty"`
	ListenPort uint16 `json:"listen_port,omitempty"`
	Sig        []byte `json:"sig"`
}

type HelloOK struct {
	Ed25519    []byte `json:"ed25519"`
	X25519     []byte `json:"x25519"`
	Name       string `json:"name,omitempty"`
	ObservedIP string `json:"observed_ip,omitempty"`
	Ts         int64  `json:"ts"`
	Sig        []byte `json:"sig"`
	CloudSeed  bool   `json:"cloud_seed,omitempty"`
}

type Presence struct {
	PubKeys [][]byte `json:"pubkeys"`
	X25519  []byte   `json:"x25519,omitempty"`
	Name    string   `json:"name,omitempty"`
}

type PresenceGossip struct {
	Hub     []byte        `json:"hub"`
	Entries []GossipEntry `json:"entries"`
	Seq     uint64        `json:"seq"`
	Bloom   []byte        `json:"bloom,omitempty"`
	Sig     []byte        `json:"sig,omitempty"`
}

type GossipEntry struct {
	Pub    []byte `json:"pub"`
	X25519 []byte `json:"x25519,omitempty"`
	Name   string `json:"name,omitempty"`
}

type Ping struct {
	T int64 `json:"t"`
}

type ErrorBody struct {
	Error string `json:"error"`
}

func WriteFrame(w io.Writer, msg Message) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(body) > MaxFrame {
		return errors.New("frame too large")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func ReadFrame(r io.Reader) (*Message, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > MaxFrame {
		return nil, errors.New("bad frame length")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	var msg Message
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func encodeBody(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func msgOf(typ string, v any) Message {
	return Message{Type: typ, Body: encodeBody(v)}
}

func nowUnix() int64 { return time.Now().Unix() }

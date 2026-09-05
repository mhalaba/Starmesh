// Package invite encodes and decodes Starmesh hub invites.
//
// A full invite (QR / paste / Meshtastic) is:
//
//	starmesh1:<crockford-base32>
//
// That blob carries version, hub Ed25519 pubkey, IPv6, optional IPv4,
// port, expiry, optional name, and a cloud-seed flag. No DNS names.
//
// A 10-character Crockford code is a fingerprint of that payload. Ten
// characters cannot hold an IPv6 address plus a 32-byte key (that would
// need ~100 characters). The short code resolves via:
//  1. last-good hub cache
//  2. signed pre-provisioned community list
//  3. a full invite that was just scanned (remembered by fingerprint)
//
// Meshtastic text frames are ~237 bytes; the full starmesh1: blob fits.
package invite

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	Version      = 1
	Prefix       = "starmesh1:"
	ShortLen     = 10
	DefaultPort  = 4433
	MaxNameBytes = 32

	FlagIPv4 = 1 << 0
	FlagSeed = 1 << 1
	FlagName = 1 << 2
)

// Crockford Base32 (no checksum digit, no padding). I, L, O, U omitted
// from the alphabet so the code can be read over radio or written down.
var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

var (
	ErrEmpty     = errors.New("invite: empty")
	ErrVersion   = errors.New("invite: unsupported version")
	ErrChecksum  = errors.New("invite: checksum mismatch")
	ErrTruncated = errors.New("invite: truncated")
	ErrNoIPv6    = errors.New("invite: hub invite requires IPv6 (Starlink hubs must listen on IPv6)")
	ErrShort     = errors.New("invite: 10-char code is a fingerprint; scan QR or paste starmesh1: blob unless the hub is already cached")
	ErrBadKey    = errors.New("invite: pubkey must be 32 bytes")
	ErrBadAddr   = errors.New("invite: invalid IP")
)

// Invite is the DNS-less locator for a hub.
type Invite struct {
	Version   uint8
	PubKey    [32]byte
	IPv6      net.IP
	IPv4      net.IP // optional; omitted when the hub is behind CGNAT
	Port      uint16
	Expiry    time.Time
	Name      string
	CloudSeed bool
}

// ShortResolver maps a 10-char fingerprint to a previously known invite.
type ShortResolver func(short string) (*Invite, bool)

func (inv Invite) ShortCode() string {
	raw, err := inv.marshal(false)
	if err != nil {
		return ""
	}
	return shortFrom(raw)
}

func (inv Invite) ShortDisplay() string {
	s := inv.ShortCode()
	if len(s) != ShortLen {
		return s
	}
	return s[:5] + "-" + s[5:]
}

func (inv Invite) Encode() (string, error) {
	raw, err := inv.marshal(true)
	if err != nil {
		return "", err
	}
	return Prefix + crockford.EncodeToString(raw), nil
}

func (inv Invite) QRPayload() (string, error) {
	return inv.Encode()
}

func (inv Invite) AddrIPv6() string {
	if inv.IPv6 == nil {
		return ""
	}
	port := inv.Port
	if port == 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("[%s]:%d", inv.IPv6.String(), port)
}

func (inv Invite) AddrIPv4() string {
	if inv.IPv4 == nil {
		return ""
	}
	port := inv.Port
	if port == 0 {
		port = DefaultPort
	}
	return fmt.Sprintf("%s:%d", inv.IPv4.String(), port)
}

func (inv Invite) Expired(now time.Time) bool {
	if inv.Expiry.IsZero() {
		return false
	}
	return now.After(inv.Expiry)
}

func (inv Invite) marshal(withCRC bool) ([]byte, error) {
	if inv.Version == 0 {
		inv.Version = Version
	}
	if inv.Version != Version {
		return nil, ErrVersion
	}
	if inv.PubKey == [32]byte{} {
		return nil, ErrBadKey
	}
	ipv6 := inv.IPv6.To16()
	if ipv6 == nil && !inv.CloudSeed {
		return nil, ErrNoIPv6
	}
	if ipv6 == nil {
		ipv6 = make([]byte, 16)
	}
	if v4 := inv.IPv6.To4(); v4 != nil && !inv.CloudSeed {
		return nil, ErrNoIPv6
	}

	var flags byte
	if inv.IPv4 != nil && inv.IPv4.To4() != nil {
		flags |= FlagIPv4
	}
	if inv.CloudSeed {
		flags |= FlagSeed
	}
	name := []byte(inv.Name)
	if len(name) > MaxNameBytes {
		name = name[:MaxNameBytes]
	}
	if len(name) > 0 {
		flags |= FlagName
	}

	port := inv.Port
	if port == 0 {
		port = DefaultPort
	}

	buf := bytes.NewBuffer(make([]byte, 0, 80))
	buf.WriteByte(inv.Version)
	buf.WriteByte(flags)
	buf.Write(inv.PubKey[:])
	buf.Write(ipv6)
	_ = binary.Write(buf, binary.BigEndian, port)
	var exp uint32
	if !inv.Expiry.IsZero() {
		exp = uint32(inv.Expiry.Unix())
	}
	_ = binary.Write(buf, binary.BigEndian, exp)
	if flags&FlagIPv4 != 0 {
		buf.Write(inv.IPv4.To4())
	}
	if flags&FlagName != 0 {
		buf.WriteByte(byte(len(name)))
		buf.Write(name)
	}
	raw := buf.Bytes()
	if withCRC {
		sum := crc32IEEE(raw)
		var crc [4]byte
		binary.BigEndian.PutUint32(crc[:], sum)
		raw = append(raw, crc[:]...)
	}
	return raw, nil
}

// Decode parses a full starmesh1: blob, a hyphenated 10-char code, or a
// raw Crockford payload. Short codes require a resolver.
func Decode(s string, resolve ShortResolver) (*Invite, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrEmpty
	}
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))

	switch {
	case strings.HasPrefix(strings.ToLower(s), Prefix):
		// restore prefix case
		payload := s[len(Prefix):]
		return decodePayload(payload)
	case strings.HasPrefix(s, "STARMESH1:"):
		return decodePayload(s[len("STARMESH1:"):])
	}

	compact := strings.ReplaceAll(s, "-", "")
	if isShortCode(compact) {
		if resolve != nil {
			if inv, ok := resolve(compact); ok && inv != nil {
				return inv, nil
			}
		}
		return nil, fmt.Errorf("%w (%s)", ErrShort, FormatShort(compact))
	}

	// Bare Crockford of a full payload (QR software sometimes strips schemes).
	return decodePayload(compact)
}

func DecodeFull(s string) (*Invite, error) {
	return Decode(s, nil)
}

func FormatShort(code string) string {
	code = strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	if len(code) != ShortLen {
		return code
	}
	return code[:5] + "-" + code[5:]
}

func NormalizeShort(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func isShortCode(s string) bool {
	if len(s) != ShortLen {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			return false
		}
	}
	return true
}

func decodePayload(s string) (*Invite, error) {
	s = strings.ToUpper(strings.ReplaceAll(s, "-", ""))
	s = normalizeCrockford(s)
	raw, err := crockford.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invite: decode: %w", err)
	}
	return unmarshal(raw)
}

func unmarshal(raw []byte) (*Invite, error) {
	if len(raw) < 2+32+16+2+4+4 {
		return nil, ErrTruncated
	}
	body, crcBytes := raw[:len(raw)-4], raw[len(raw)-4:]
	want := binary.BigEndian.Uint32(crcBytes)
	if crc32IEEE(body) != want {
		return nil, ErrChecksum
	}
	ver := body[0]
	if ver != Version {
		return nil, ErrVersion
	}
	flags := body[1]
	off := 2
	var inv Invite
	inv.Version = ver
	copy(inv.PubKey[:], body[off:off+32])
	off += 32
	inv.IPv6 = net.IP(append([]byte(nil), body[off:off+16]...))
	off += 16
	inv.Port = binary.BigEndian.Uint16(body[off : off+2])
	off += 2
	exp := binary.BigEndian.Uint32(body[off : off+4])
	off += 4
	if exp != 0 {
		inv.Expiry = time.Unix(int64(exp), 0).UTC()
	}
	if flags&FlagIPv4 != 0 {
		if off+4 > len(body) {
			return nil, ErrTruncated
		}
		inv.IPv4 = net.IPv4(body[off], body[off+1], body[off+2], body[off+3])
		off += 4
	}
	if flags&FlagName != 0 {
		if off >= len(body) {
			return nil, ErrTruncated
		}
		n := int(body[off])
		off++
		if off+n > len(body) {
			return nil, ErrTruncated
		}
		inv.Name = string(body[off : off+n])
	}
	inv.CloudSeed = flags&FlagSeed != 0
	if inv.IPv6.Equal(net.IPv6zero) {
		inv.IPv6 = nil
	}
	if !inv.CloudSeed && (inv.IPv6 == nil || inv.IPv6.To4() != nil) {
		return nil, ErrNoIPv6
	}
	return &inv, nil
}

func shortFrom(raw []byte) string {
	h := sha256.Sum256(raw)
	s := crockford.EncodeToString(h[:8])
	if len(s) > ShortLen {
		s = s[:ShortLen]
	}
	return s
}

func normalizeCrockford(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		switch c {
		case 'I', 'L':
			b.WriteByte('1')
		case 'O':
			b.WriteByte('0')
		case 'U':
			b.WriteByte('V')
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}

// IEEE CRC-32 (ISO 3309). Kept local so invite has no extra deps.
var crcTable = makeIEEETable()

func makeIEEETable() [256]uint32 {
	var t [256]uint32
	const poly = 0xEDB88320
	for i := 0; i < 256; i++ {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = poly ^ (crc >> 1)
			} else {
				crc >>= 1
			}
		}
		t[i] = crc
	}
	return t
}

func crc32IEEE(p []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range p {
		crc = crcTable[byte(crc)^b] ^ (crc >> 8)
	}
	return crc ^ 0xFFFFFFFF
}

// LookupTable is a ShortResolver over a static list.
func LookupTable(invites []*Invite) ShortResolver {
	m := make(map[string]*Invite, len(invites))
	for _, inv := range invites {
		if inv == nil {
			continue
		}
		m[inv.ShortCode()] = inv
	}
	return func(short string) (*Invite, bool) {
		inv, ok := m[NormalizeShort(short)]
		return inv, ok
	}
}

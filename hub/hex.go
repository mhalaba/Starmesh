package hub

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

var crockford = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

type hexBytes []byte

func (h hexBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(h))
}

func (h *hexBytes) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	*h = raw
	return nil
}

func sha256Short(p []byte) string {
	sum := sha256.Sum256(p)
	s := crockford.EncodeToString(sum[:8])
	if len(s) > 10 {
		s = s[:10]
	}
	if len(s) == 10 {
		return s[:5] + "-" + s[5:]
	}
	return s
}

func mustHex(p []byte) string {
	return hex.EncodeToString(p)
}

func parseHex(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("hex: %w", err)
	}
	return b, nil
}

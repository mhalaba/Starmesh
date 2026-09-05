package hub

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
)

// CommunityFile is a signed list of pre-provisioned hub locators.
// The signature is Ed25519 over SHA-256 of canonical JSON of the hubs array.
type CommunityFile struct {
	Version int         `json:"version"`
	RootPub hexBytes    `json:"root_pubkey"`
	Hubs    []CachedHub `json:"hubs"`
	Sig     hexBytes    `json:"sig"`
}

func LoadCommunity(path string, pinnedRoot ed25519.PublicKey) (*CommunityFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f CommunityFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if err := f.Verify(pinnedRoot); err != nil {
		return nil, err
	}
	for i := range f.Hubs {
		if f.Hubs[i].Source == "" {
			f.Hubs[i].Source = SrcCommunity
		}
		if inv := f.Hubs[i].Invite(); inv != nil {
			f.Hubs[i].Short = inv.ShortDisplay()
		}
	}
	return &f, nil
}

func (f *CommunityFile) Verify(pinned ed25519.PublicKey) error {
	if len(f.RootPub) != ed25519.PublicKeySize {
		return fmt.Errorf("community: bad root pubkey")
	}
	if pinned != nil && !bytes.Equal(pinned, f.RootPub) {
		return fmt.Errorf("community: root pubkey does not match built-in pin")
	}
	msg, err := canonicalHubs(f.Hubs)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(msg)
	if !ed25519.Verify(ed25519.PublicKey(f.RootPub), sum[:], f.Sig) {
		return fmt.Errorf("community: bad signature")
	}
	return nil
}

func SignCommunity(root ed25519.PrivateKey, hubs []CachedHub) (*CommunityFile, error) {
	msg, err := canonicalHubs(hubs)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(msg)
	return &CommunityFile{
		Version: 1,
		RootPub: hexBytes(root.Public().(ed25519.PublicKey)),
		Hubs:    hubs,
		Sig:     hexBytes(ed25519.Sign(root, sum[:])),
	}, nil
}

func canonicalHubs(hubs []CachedHub) ([]byte, error) {
	type wire struct {
		Name      string `json:"name,omitempty"`
		Ed25519   string `json:"ed25519"`
		X25519    string `json:"x25519,omitempty"`
		IPv6      string `json:"ipv6,omitempty"`
		IPv4      string `json:"ipv4,omitempty"`
		Port      uint16 `json:"port"`
		CloudSeed bool   `json:"cloud_seed,omitempty"`
	}
	w := make([]wire, 0, len(hubs))
	for _, h := range hubs {
		w = append(w, wire{
			Name:      h.Name,
			Ed25519:   mustHex(h.Ed25519),
			X25519:    mustHex(h.X25519),
			IPv6:      h.IPv6,
			IPv4:      h.IPv4,
			Port:      h.Port,
			CloudSeed: h.CloudSeed,
		})
	}
	return json.Marshal(w)
}

func (f *CommunityFile) Save(path string) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

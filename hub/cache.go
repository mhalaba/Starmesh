package hub

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/mhalaba/Starmesh/invite"
)

type HubSource string

const (
	SrcOpen      HubSource = "open"
	SrcCache     HubSource = "cache"
	SrcCommunity HubSource = "community"
	SrcInvite    HubSource = "invite"
	SrcCloud     HubSource = "cloud"
)

// CachedHub is a last-good locator. No hostnames.
type CachedHub struct {
	Name      string    `json:"name,omitempty"`
	Ed25519   hexBytes  `json:"ed25519"`
	X25519    hexBytes  `json:"x25519,omitempty"`
	IPv6      string    `json:"ipv6,omitempty"`
	IPv4      string    `json:"ipv4,omitempty"`
	Port      uint16    `json:"port"`
	LastRTTMs int64     `json:"last_rtt_ms,omitempty"`
	LastSeen  time.Time `json:"last_seen"`
	Source    HubSource `json:"source,omitempty"`
	CloudSeed bool      `json:"cloud_seed,omitempty"`
	Short     string    `json:"short,omitempty"`
}

type Cache struct {
	mu   sync.Mutex
	path string
	Hubs []CachedHub `json:"hubs"`
}

func OpenCache(path string) *Cache {
	c := &Cache{path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, c)
	}
	return c
}

func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, 0o600)
}

func (c *Cache) Remember(h CachedHub) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h.Port == 0 {
		h.Port = invite.DefaultPort
	}
	h.LastSeen = time.Now().UTC()
	key := string(h.Ed25519)
	for i, x := range c.Hubs {
		if string(x.Ed25519) == key {
			if h.LastRTTMs == 0 {
				h.LastRTTMs = x.LastRTTMs
			}
			if h.Source == "" {
				h.Source = x.Source
			}
			c.Hubs[i] = h
			return
		}
	}
	c.Hubs = append(c.Hubs, h)
}

func (c *Cache) List() []CachedHub {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]CachedHub, len(c.Hubs))
	copy(out, c.Hubs)
	return out
}

func (c *Cache) Resolver() invite.ShortResolver {
	c.mu.Lock()
	defer c.mu.Unlock()
	var invs []*invite.Invite
	for i := range c.Hubs {
		if inv := c.Hubs[i].Invite(); inv != nil {
			invs = append(invs, inv)
		}
	}
	return invite.LookupTable(invs)
}

func (h CachedHub) Invite() *invite.Invite {
	if len(h.Ed25519) != 32 {
		return nil
	}
	inv := &invite.Invite{Version: invite.Version, Port: h.Port, Name: h.Name, CloudSeed: h.CloudSeed}
	copy(inv.PubKey[:], h.Ed25519)
	if h.IPv6 != "" {
		inv.IPv6 = net.ParseIP(h.IPv6)
	}
	if h.IPv4 != "" {
		inv.IPv4 = net.ParseIP(h.IPv4)
	}
	if inv.IPv6 == nil && !inv.CloudSeed {
		return nil
	}
	return inv
}

func (h CachedHub) Priority() int {
	if h.CloudSeed || h.Source == SrcCloud {
		return 5
	}
	switch h.Source {
	case SrcOpen:
		return 1
	case SrcCache:
		return 2
	case SrcCommunity:
		return 3
	case SrcInvite:
		return 4
	default:
		return 2
	}
}

// DialOrder implements the spec:
//  1. already-open (caller inserts)
//  2. cached hubs, lowest RTT first
//  3. pre-provisioned community hubs
//  4. fresh invite just scanned
//  5. cloud seeds
func DialOrder(open []CachedHub, cached []CachedHub, community []CachedHub, fresh []CachedHub, seeds []CachedHub) []CachedHub {
	seen := map[string]struct{}{}
	var out []CachedHub
	add := func(list []CachedHub, src HubSource) {
		cp := append([]CachedHub(nil), list...)
		sort.SliceStable(cp, func(i, j int) bool {
			if src == SrcCache || src == SrcOpen {
				if cp[i].LastRTTMs != cp[j].LastRTTMs {
					if cp[i].LastRTTMs == 0 {
						return false
					}
					if cp[j].LastRTTMs == 0 {
						return true
					}
					return cp[i].LastRTTMs < cp[j].LastRTTMs
				}
			}
			return cp[i].LastSeen.After(cp[j].LastSeen)
		})
		for _, h := range cp {
			k := string(h.Ed25519) + "|" + h.IPv6 + "|" + h.IPv4
			if _, ok := seen[k]; ok {
				continue
			}
			if h.Source == "" {
				h.Source = src
			}
			seen[k] = struct{}{}
			out = append(out, h)
		}
	}
	add(open, SrcOpen)
	var cacheNoSeed, cacheSeed []CachedHub
	for _, h := range cached {
		if h.CloudSeed {
			cacheSeed = append(cacheSeed, h)
		} else {
			cacheNoSeed = append(cacheNoSeed, h)
		}
	}
	add(cacheNoSeed, SrcCache)
	add(community, SrcCommunity)
	add(fresh, SrcInvite)
	add(seeds, SrcCloud)
	add(cacheSeed, SrcCloud)
	return out
}

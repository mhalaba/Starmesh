package hub

import (
	"net"
	"testing"

	"github.com/mhalaba/Starmesh/invite"
)

func TestDialOrderCloudLast(t *testing.T) {
	pk := func(b byte) hexBytes {
		x := make([]byte, 32)
		x[0] = b
		return x
	}
	open := []CachedHub{{Ed25519: pk(1), IPv6: "::1", Source: SrcOpen}}
	cached := []CachedHub{
		{Ed25519: pk(2), IPv6: "::2", LastRTTMs: 80, Source: SrcCache},
		{Ed25519: pk(3), IPv6: "::3", LastRTTMs: 20, Source: SrcCache},
		{Ed25519: pk(9), IPv6: "::9", CloudSeed: true, LastRTTMs: 1},
	}
	community := []CachedHub{{Ed25519: pk(4), IPv6: "::4"}}
	fresh := []CachedHub{{Ed25519: pk(5), IPv6: "::5"}}
	seeds := []CachedHub{{Ed25519: pk(6), IPv6: "::6", CloudSeed: true}}
	got := DialOrder(open, cached, community, fresh, seeds)
	var keys []byte
	for _, h := range got {
		keys = append(keys, h.Ed25519[0])
	}
	// 1 open, 3 then 2 by RTT, 4 community, 5 fresh, then seeds 6 and cached-seed 9
	want := []byte{1, 3, 2, 4, 5, 6, 9}
	if string(keys) != string(want) {
		t.Fatalf("order %v want %v", keys, want)
	}
}

func TestCacheResolverShortCode(t *testing.T) {
	id, _ := GenerateIdentity("hub")
	c := OpenCache("")
	inv := &invite.Invite{Version: 1, IPv6: net.ParseIP("::1"), Port: 4433, Name: "OSP"}
	copy(inv.PubKey[:], id.EdPub)
	blob, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := invite.DecodeFull(blob)
	c.Remember(cachedFromInvite(parsed, SrcInvite))
	got, err := invite.Decode(parsed.ShortDisplay(), c.Resolver())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "OSP" {
		t.Fatal(got.Name)
	}
}

# Starlink networking reality (Starmesh v2)

This overlay is designed around what Starlink actually does, not around
a wish for a full peer mesh.

## IPv4 is CGNAT. Spokes cannot listen.

Residential Starlink hands the customer a **CGNAT** IPv4 address in
`100.64.0.0/10` (or the LAN behind the stock router is RFC1918). There
is no inbound IPv4 hole. Two phones on two dishes cannot dial each
other on IPv4. A "full mesh" of spokes over IPv4 is not a thing.

Starmesh spokes are **outbound only**. They open QUIC (or TCP/TLS) to a
hub. They never bind a public port.

## IPv6 is the real Starlink network

Every customer gets a public IPv6 **/56**. Traffic from one Starlink
terminal to another stays in **AS14593** and does not need the
terrestrial public internet backbone. That is why a hub on a dish in
Nadarzyn can still relay Warsaw↔Kraków when the EU backbone is sad.

A hub **must listen on UDP/QUIC IPv6**. IPv4 is optional and only
advertised when the station actually has a public IPv4 (Starlink
Priority). If the WAN address is CGNAT or RFC1918, Starmesh will not
claim IPv4 reachability — even if some STUN oracle saw a mapping,
because that mapping is not a stable inbound listener.

## The stock router firewalls inbound IPv6

The consumer Starlink router NATs IPv4 and **filters inbound IPv6**.
A process on a phone or laptop behind that router cannot be a hub.

A hub has to be one of:

1. A Linux mini-PC (or third-party router) on the dish in **bypass
   mode**, with an IPv6 allowlist for UDP/TCP 4433, **or**
2. A dish with **Starlink Priority** public IPv4 (still listen on IPv6
   too).

See `/deploy` for the mini-PC path.

## Roles

| Role | Who | Inbound |
|------|-----|---------|
| Spoke (default) | Family phones, laptops, most dishes | None. Outbound to hubs. Queue locally if nobody answers. |
| Hub | Pre-agreed community installs; or a human tapping **Become hub** on a capable station after an outage | UDP/QUIC IPv6 required; IPv4 only if public |
| Cloud seed | Same binary on a VPS in another continent | Last resort. Not required for the EU-backbone-down scenario. |

## Discovery without DNS

DNS is a terrestrial luxury. Starmesh locates hubs with:

1. Last-good cache (IP, key, last seen, RTT)
2. Invite QR + `starmesh1:` blob (version, hub pubkey, IPv6, optional IPv4, port, expiry, name)
3. 10-character Crockford fingerprint of that blob (radio / paper). It
   does **not** contain the IPv6 address — 10 chars is 50 bits. It
   resolves against cache, the signed community list, or a QR you just
   scanned.
4. Manual paste of IPv6 + pubkey
5. mDNS / LAN multicast **only on the same Starlink LAN** (family phones)
6. Meshtastic beacon of the invite (interface stub in MVP; the full
   blob fits in a Meshtastic text frame)

No hostname is ever required.

## Connection order

1. Already-open hub session
2. Cached hubs, lowest RTT first (dead hub deprioritized)
3. Pre-provisioned community hubs (signed file)
4. Fresh invite just scanned
5. Cloud seeds

Timeouts are 15–20 seconds. Starlink is lossy and prefixes move.

## Become hub

The app probes:

- Global IPv6 (`2000::/3`) on this host?
- Can we bind UDP :4433?
- Public IPv4 only if a local address equals the observed WAN IPv4.
  `100.64/10` and RFC1918 ⇒ do not claim IPv4.

If that fails:

> This dish cannot be a hub. Put the app on a PC behind a bypass router or use a Priority public IP.

`--dev` relaxes the probe to loopback/ULA so labs can run without a dish.

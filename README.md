# Starmesh v2 — Starlink hub-and-spoke overlay

Primary network is **not** a cloud VPS. It is a hub-and-spoke overlay that
runs **on Starlink terminals**. Cloud seeds exist, last in the dial order,
and are not required when the EU backbone is down.

```
  spoke (phone, CGNAT) --outbound QUIC/IPv6-->  hub (bypass mini-PC)
  spoke (phone, CGNAT) --outbound QUIC/IPv6-->  hub (Priority dish)
                         hubs peer with hubs
  cloud seed VPS  <only if no Starlink hub answers>
```

## Reality this repo does not fight

| Fact | Consequence |
|------|-------------|
| Residential Starlink IPv4 is CGNAT | Spokes cannot accept inbound IPv4. Full peer mesh over IPv4 is impossible. |
| Every customer gets a public IPv6 /56 | Starlink-to-Starlink stays in AS14593. Hubs listen on IPv6. |
| Stock router firewalls inbound IPv6 | A hub must sit behind **bypass mode** (third-party router) with an IPv6 allowlist, **or** have Priority public IPv4. |
| Most installs are spokes | A few pre-agreed community sites are hubs. After an outage a human taps **Become hub** on a capable station. |
| No DNS in a backbone-down world | Invites are binary locators: QR + `starmesh1:` blob + 10-char fingerprint. |

See [docs/starlink.md](docs/starlink.md).

## Layout

| Path | What |
|------|------|
| `/invite` | Encode/decode library (Go + Dart). No DNS names. |
| `/hub` | Same Go binary for a Starlink hub and a cloud seed. CLI spoke included. |
| `/app` | Flutter UI (Network, chat, Become hub, full-screen QR). |
| `/deploy` | Linux mini-PC behind Starlink bypass. |
| `/docs/starlink.md` | CGNAT vs IPv6 vs Priority. |

## Quick lab (no dish)

```bash
go test ./...
go build -o dist/starmesh ./hub/cmd/starmesh

# terminal 1 — this host has no global IPv6, so --dev is required
./dist/starmesh hub --dev --name "OSP Nadarzyn"

# terminal 2
./dist/starmesh spoke --name alice --invite 'starmesh1:...' --home /tmp/alice

# terminal 3
./dist/starmesh spoke --name bob --invite 'starmesh1:...' --home /tmp/bob
```

`--dev` lets a machine without global IPv6 become a hub on `::1`. A real
dish without bypass still prints:

> This dish cannot be a hub. Put the app on a PC behind a bypass router or use a Priority public IP.

## Connection order

1. Any already-open hub session
2. Cached hubs, lowest RTT first
3. Pre-provisioned community hubs (signed config)
4. Fresh invite just scanned
5. Cloud seeds

Timeouts are 15–20s (Starlink is lossy and IPs move).

## Crypto

- Identity: Ed25519 per install
- Boxes: X25519 (NaCl box). Relays never see plaintext.
- Transport: QUIC first (`starmesh/1`), TCP/TLS 1.3 fallback
- Envelope: `id, from, to, ts, type, sig, ciphertext`

## Become hub

The probe checks: global IPv6 on this host, UDP :4433 bind, public IPv4
only if WAN IP equals the observed IPv4. `100.64/10` and RFC1918 are never
advertised as reachable IPv4.

## Flutter

```bash
cd app && flutter create . --project-name starmesh && flutter run
```

The UI talks to the Go process on `127.0.0.1:7780` (hub `--api`).
Phones remain spokes: outbound only.

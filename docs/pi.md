# Run on Raspberry Pi 5 (Starlink lab)

Primary operator UI is the **web panel inside the Go binary**. Do not
install Flutter, Python, or Docker on the Pi.

This host today (typical TASAI lab):

| | |
|--|--|
| Board | Raspberry Pi 5 8GB, Raspberry Pi OS 64-bit |
| User / LAN | `tas` @ `10.1.1.209/24`, gateway `10.1.1.1` |
| Uplink | Starlink: outbound IPv4 works |
| IPv6 | only `fe80::` on wlan0 — **no global IPv6 yet** |

`--dev` keeps the lab alive without a global `/56`. A real Starlink hub
still needs global IPv6, RA/PD or dish **bypass**, and UDP/TCP **4433**
allowlisted. See [starlink.md](starlink.md).

Cloud seed `168.138.14.210:4433` is QUIC/TLS, **last** in dial order.
Never treat it as a web host.

## 1. Build `linux/arm64`

On a desktop (or on the Pi if Go is installed):

```bash
GOOS=linux GOARCH=arm64 go test ./...
GOOS=linux GOARCH=arm64 go build -o dist/starmesh-linux-arm64 ./hub/cmd/starmesh
scp dist/starmesh-linux-arm64 tas@10.1.1.209:~/bin/starmesh
```

On the Pi:

```bash
mkdir -p ~/bin ~/.starmesh/hub ~/.starmesh/spoke
chmod 700 ~/.starmesh ~/.starmesh/hub ~/.starmesh/spoke
```

Hub and spoke on **one** Pi must use different `--home` (or they share
one Ed25519 fingerprint). Defaults:

| Process | `--home` |
|---------|----------|
| `node` / `hub` | `~/.starmesh` (operator chat identity: `~/.starmesh/operator`) |
| extra spoke on the same Pi | `~/.starmesh/spoke` |
| env overrides | `STARMESH_HOME`, `STARMESH_HUB_HOME` (docs only — pass `--home`) |

## 2. One process (recommended): `starmesh node`

Headless. No stdin. Web UI + JSON API + hub + local chat identity.

Without global IPv6:

```bash
~/bin/starmesh node --dev --name "tasai-lab" \
  --home ~/.starmesh/hub \
  --api 0.0.0.0:7780
```

Open the panel from a phone or laptop on `10.1.1.0/24`:

```
http://10.1.1.209:7780/
```

Optional LAN token (`Authorization: Bearer` / `X-Starmesh-Token` / `?token=`):

```bash
export STARMESH_API_TOKEN=change-me
~/bin/starmesh node --dev --name "tasai-lab" \
  --home ~/.starmesh/hub \
  --api 0.0.0.0:7780 \
  --api-token "$STARMESH_API_TOKEN"
# then: http://10.1.1.209:7780/?token=change-me
```

`--dev` listens on all interfaces and may put the Pi’s RFC1918 IPv4
(e.g. `10.1.1.209`) on the invite so another LAN host can dial **before**
global IPv6 exists. That IPv4 is **not** claimed as a public Starlink
address.

## 3. systemd user unit

```bash
mkdir -p ~/.config/systemd/user ~/.config/starmesh
cp deploy/user/starmesh.service ~/.config/systemd/user/
cp deploy/starmesh.env.example ~/.config/starmesh/starmesh.env
# edit names / token / --dev
systemctl --user daemon-reload
systemctl --user enable --now starmesh
journalctl --user -u starmesh -f
```

Linger so it survives logout: `sudo loginctl enable-linger tas`.

System-wide units: `deploy/starmesh.service` (node),
`deploy/starmesh-hub.service`, `deploy/starmesh-spoke.service`.

`spoke --api` no longer dies when stdin is `/dev/null`.

## 4. Second spoke (phone, another Pi, or same Pi)

**Same Pi (lab dual-process):**

```bash
# after the node is up, copy the starmesh1: blob from the web QR page
~/bin/starmesh spoke --headless --name alice \
  --home ~/.starmesh/spoke \
  --invite 'starmesh1:...' \
  --api 127.0.0.1:7781
```

Chat: open `http://10.1.1.209:7780/`, pick **alice** on Czat, send.
Or use alice’s panel on `:7781` (loopback only in this example).

**Another device on the LAN:** same `--invite` (includes lab IPv4 under
`--dev`). Phones stay outbound-only spokes.

**After global IPv6:** drop `--dev`, put the dish in bypass, advertise
the `/56` (radvd), allow UDP/TCP 4433. Invites then carry real IPv6.

## 5. Flutter (optional, not on the Pi)

The same `/v1/*` API is used by Flutter. Pi docs lead with the web
panel. See [flutter.md](flutter.md).

## Dial order (unchanged)

1. Open hub session  
2. Cached hubs (lowest RTT)  
3. Community list  
4. Fresh invite  
5. Cloud seeds  

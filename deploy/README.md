# Run a Starmesh hub on a Linux mini-PC behind Starlink bypass

The stock Starlink router must be in **bypass / passthrough**. The
mini-PC (or OpenWrt box) is then the router: it gets the public IPv6
/56 and, on residential, a CGNAT IPv4. Starmesh will listen on IPv6
and will **not** advertise IPv4.

## 1. Bypass

Starlink app → bypass mode. Cable from the dish/router LAN to the
mini-PC WAN (or a single Ethernet if you use the dish's LAN port in
bypass). Confirm:

```bash
ip -6 addr
# expect a 2a0d:… or 2406:… global address, not just fe80:
ip -4 addr
# 100.64/10 = CGNAT. Do not port-forward IPv4. Do not claim it.
```

## 2. IPv6 firewall allowlist

UDP and TCP **4433** from the Starlink IPv6 prefix (or `::/0` if you
accept that every AS14593 peer may hit the hub).

nftables sketch:

```
table inet filter {
  chain input {
    type filter hook input priority 0;
    iif lo accept
    ct state established,related accept
    ip6 nexthdr udp udp dport 4433 accept
    ip6 nexthdr tcp tcp dport 4433 accept
    icmpv6 type { nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert, echo-request } accept
    drop
  }
}
```

## 3. Prefix advertisement (RA)

Once the stock router is in bypass, **it no longer speaks RA** to your
LAN. Phones on Wi-Fi need a global IPv6 to open outbound sessions to
other dishes. Run `radvd` (or systemd-networkd `IPv6SendRA=`).

See `radvd.conf` in this directory.

## 4. Install the binary

```bash
go build -o /usr/local/bin/starmesh ./hub/cmd/starmesh
install -m 644 deploy/starmesh.service /etc/systemd/system/starmesh.service
# edit User= and --name
systemctl daemon-reload
systemctl enable --now starmesh
journalctl -u starmesh -f
```

The unit prints a QR and a 10-char code on stdout (captured by
journald). `starmesh become-hub` is the same binary; it refuses if this
host has no global IPv6.

## 5. Cloud seed (optional, last resort)

Same binary on a VPS **in another continent**:

```
starmesh hub --cloud-seed --name "seed-ewr" --force
```

Spokes try it only after Starlink hubs are silent. Do not make the EU
outage plan depend on it.

### 5a. Free hosting for the first seed

You do not need to pay for the bootstrap seed. Two forever-free paths
live in [`cloud-seed/`](cloud-seed):

**Oracle Cloud "Always Free"** (recommended — public IPv4 *and* routable
IPv6, no time limit). Create a `VM.Standard.A1.Flex` or `E2.1.Micro`
Ubuntu instance and paste [`cloud-seed/cloud-init.yaml`](cloud-seed/cloud-init.yaml)
into the cloud-init box. It installs Go, builds the binary, creates a
`starmesh` user, opens UDP+TCP 4433 (ufw), and starts
[`starmesh-seed@.service`](cloud-seed/starmesh-seed@.service). Also open
4433 in the Oracle **Security List** (that firewall sits in front of
the VM). Grab the invite with:

```bash
journalctl -u 'starmesh-seed@*' --no-pager | grep -A3 starmesh1:
```

**Fly.io** (free dedicated IPv6). From the repo root:

```bash
fly launch --no-deploy --copy-config --config deploy/cloud-seed/fly/fly.toml
fly volume create starmesh_data --size 1 --region fra
fly ips allocate-v6
fly deploy --config deploy/cloud-seed/fly/fly.toml \
           --dockerfile deploy/cloud-seed/fly/Dockerfile .
fly logs | grep -A3 starmesh1:
```

A push-button deploy also lives in
[`.github/workflows/deploy-seed.yml`](../.github/workflows/deploy-seed.yml)
— add a `FLY_API_TOKEN` repo secret and run the workflow.

Any plain Ubuntu VPS (Hetzner, GCP e2-micro, etc.) works with the same
cloud-init file.

## 6. Community list (default broadcasting source)

The app ships a **signed** community list so the seed is trusted without
scanning a QR. The trust root is pinned; the current root is in
[`cloud-seed/community-root.pub`](cloud-seed/community-root.pub).

After the seed prints its live invite, regenerate the signed list from
that blob (it carries the real IPv6):

```bash
ROOT_HOME=$HOME/.starmesh-community \
  deploy/cloud-seed/gen-community.sh 'starmesh1:...'
# copy the result into the Flutter bundle:
cp deploy/cloud-seed/community-hubs.json app/assets/community-hubs.json
```

Keep `ROOT_HOME` (the community-root private key) offline and backed up —
it is the trust anchor pinned in every client.

Distribution / consumption:

- **Flutter app** — bundles `app/assets/community-hubs.json`; the Network
  tab shows the seed under *Community seeds*.
- **Daemon / CLI** — a spoke auto-loads `$STARMESH_HOME/community-hubs.json`
  when `--community` is not passed, so the seed is the default fallback.
  Override with `starmesh spoke --community /path/to/community-hubs.json`.

Short codes in that file resolve without a QR.

Manual signing (custom list):

```bash
starmesh community sign -in hubs.json -out /etc/starmesh/community-hubs.json
```

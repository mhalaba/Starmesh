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

## 6. Community list

`starmesh community sign -in hubs.json -out /etc/starmesh/community-hubs.json`

Distribute the signed file with the app. Spokes use `--community` /
the Flutter asset. Short codes in that file resolve without a QR.

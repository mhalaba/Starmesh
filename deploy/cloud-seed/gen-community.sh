#!/usr/bin/env bash
# Regenerate the signed community-hubs.json from one or more live seed invites.
#
# After a seed boots it prints a `starmesh1:...` invite (with its real IPv6).
# Feed those blobs here to produce a freshly signed community list that spokes
# and the Flutter app trust via the pinned community root.
#
# Usage:
#   deploy/cloud-seed/gen-community.sh 'starmesh1:AAAA...' ['starmesh1:BBBB...' ...]
#
# Env:
#   STARMESH_BIN   path to the starmesh binary (default: ./dist/starmesh)
#   ROOT_HOME      dir holding the community-root identity
#                  (default: $HOME/.starmesh-community). Keep this PRIVATE and
#                  backed up: it is the trust root pinned in the app.
#   OUT            output path (default: deploy/cloud-seed/community-hubs.json)
#   IPV4_OVERRIDE  optional comma-separated public IPv4s, one per invite, for
#                  1:1-NAT seeds whose invite has an empty IPv4 field.
set -euo pipefail

BIN="${STARMESH_BIN:-./dist/starmesh}"
ROOT_HOME="${ROOT_HOME:-$HOME/.starmesh-community}"
OUT="${OUT:-deploy/cloud-seed/community-hubs.json}"

if [ "$#" -lt 1 ]; then
  echo "usage: $0 'starmesh1:INVITE' [more invites...]" >&2
  exit 2
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
hubs="$tmp/hubs.json"

python3 - "$BIN" "${IPV4_OVERRIDE:-}" "$hubs" "$@" <<'PY'
import json, subprocess, sys

bin_path = sys.argv[1]
overrides = [x.strip() for x in sys.argv[2].split(",") if x.strip()]
out_path = sys.argv[3]
invites = sys.argv[4:]
hubs = []
for i, blob in enumerate(invites):
    dec = json.loads(subprocess.check_output([bin_path, "invite", "decode", blob], text=True))
    pk = dec.get("PubKey") or dec.get("pubkey") or []
    ed = bytes(pk).hex() if isinstance(pk, list) else str(pk)
    ipv4 = dec.get("IPv4") or dec.get("ipv4") or ""
    if not ipv4 and i < len(overrides):
        ipv4 = overrides[i]
    ipv6 = dec.get("IPv6") or dec.get("ipv6") or ""
    rec = {
        "name": dec.get("Name") or dec.get("name") or "",
        "ed25519": ed,
        "port": int(dec.get("Port") or dec.get("port") or 4433),
        "cloud_seed": bool(dec.get("CloudSeed") if "CloudSeed" in dec else dec.get("cloud_seed", True)),
    }
    if ipv6:
        rec["ipv6"] = ipv6
    if ipv4:
        rec["ipv4"] = ipv4
    hubs.append(rec)
open(out_path, "w").write(json.dumps(hubs, indent=2) + "\n")
print(open(out_path).read())
PY

mkdir -p "$ROOT_HOME"
"$BIN" community sign --home "$ROOT_HOME" -in "$hubs" -out "$OUT"
echo "wrote $OUT"
echo "copy it to app/assets/community-hubs.json for the Flutter bundle."

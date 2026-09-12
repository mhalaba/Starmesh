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
echo "[" > "$hubs"
first=1
for blob in "$@"; do
  # invite decode emits JSON with name/ipv6/ipv4/port/pubkey/cloud_seed.
  dec="$("$BIN" invite decode "$blob")"
  name=$(echo "$dec"   | sed -n 's/.*"name" *: *"\([^"]*\)".*/\1/p')
  ipv6=$(echo "$dec"   | sed -n 's/.*"ipv6" *: *"\([^"]*\)".*/\1/p')
  ipv4=$(echo "$dec"   | sed -n 's/.*"ipv4" *: *"\([^"]*\)".*/\1/p')
  port=$(echo "$dec"   | sed -n 's/.*"port" *: *\([0-9]*\).*/\1/p')
  ed=$(echo "$dec"     | sed -n 's/.*"pubkey" *: *"\([0-9a-f]*\)".*/\1/p')
  [ "$first" -eq 1 ] || echo "," >> "$hubs"
  first=0
  {
    echo "  {"
    echo "    \"name\": \"${name}\","
    echo "    \"ed25519\": \"${ed}\","
    [ -n "$ipv6" ] && echo "    \"ipv6\": \"${ipv6}\","
    [ -n "$ipv4" ] && echo "    \"ipv4\": \"${ipv4}\","
    echo "    \"port\": ${port:-4433},"
    echo "    \"cloud_seed\": true"
    echo -n "  }"
  } >> "$hubs"
done
echo "" >> "$hubs"
echo "]" >> "$hubs"

mkdir -p "$ROOT_HOME"
"$BIN" community sign --home "$ROOT_HOME" -in "$hubs" -out "$OUT"
echo "wrote $OUT"
echo "copy it to app/assets/community-hubs.json for the Flutter bundle."

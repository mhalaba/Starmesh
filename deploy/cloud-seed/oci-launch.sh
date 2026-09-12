#!/usr/bin/env bash
# Launch the Starmesh Always Free cloud seed on Oracle Cloud (ap-melbourne-1).
#
# Prerequisites: a working `oci` CLI profile (session or API key).
# Usage:
#   OCI_PROFILE=starmesh deploy/cloud-seed/oci-launch.sh
set -euo pipefail

export PATH="${HOME}/.local/bin:${PATH}"
PROFILE="${OCI_PROFILE:-starmesh}"
REGION="${OCI_REGION:-ap-melbourne-1}"
TENANCY="${OCI_TENANCY:-ocid1.tenancy.oc1..aaaaaaaamqdcthohtimdwkn7iwbazmal3vrbwbfcg2ikdajbdy33f2kipmeq}"
COMPARTMENT="${OCI_COMPARTMENT:-$TENANCY}"
NAME="${SEED_NAME:-seed-mel}"
SHAPE="${SEED_SHAPE:-VM.Standard.E2.1.Micro}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/starmesh_oracle.pub}"
CLOUDINIT="${CLOUDINIT:-$(cd "$(dirname "$0")" && pwd)/cloud-init.yaml}"

oci() { command oci --profile "$PROFILE" --region "$REGION" "$@"; }

echo "== auth check =="
oci iam region list --query 'data[0].name' --raw-output >/dev/null
echo "ok profile=$PROFILE region=$REGION"

if [ ! -f "$SSH_KEY" ]; then
  mkdir -p "$(dirname "$SSH_KEY")"
  ssh-keygen -t ed25519 -N "" -f "${SSH_KEY%.pub}" -C "starmesh-seed"
fi

echo "== availability domain =="
AD=$(oci iam availability-domain list --compartment-id "$COMPARTMENT" \
  --query 'data[0].name' --raw-output)
echo "AD=$AD"

echo "== Ubuntu image =="
IMAGE=$(oci compute image list --compartment-id "$COMPARTMENT" \
  --operating-system "Canonical Ubuntu" --operating-system-version "24.04" \
  --shape "$SHAPE" --sort-by TIMECREATED --sort-order DESC \
  --query 'data[0].id' --raw-output)
echo "IMAGE=$IMAGE"

echo "== VCN =="
VCN=$(oci network vcn list --compartment-id "$COMPARTMENT" --display-name starmesh-vcn \
  --query 'data[0].id' --raw-output 2>/dev/null || true)
if [ -z "$VCN" ] || [ "$VCN" = "null" ]; then
  VCN=$(oci network vcn create --compartment-id "$COMPARTMENT" \
    --display-name starmesh-vcn --cidr-block 10.0.0.0/16 \
    --dns-label starmesh --wait-for-state AVAILABLE \
    --query 'data.id' --raw-output)
fi
echo "VCN=$VCN"

echo "== Internet gateway =="
IGW=$(oci network internet-gateway list --compartment-id "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[0].id' --raw-output 2>/dev/null || true)
if [ -z "$IGW" ] || [ "$IGW" = "null" ]; then
  IGW=$(oci network internet-gateway create --compartment-id "$COMPARTMENT" \
    --vcn-id "$VCN" --is-enabled true --display-name starmesh-igw \
    --wait-for-state AVAILABLE --query 'data.id' --raw-output)
fi

RT=$(oci network route-table list --compartment-id "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[0].id' --raw-output)
oci network route-table update --rt-id "$RT" --force \
  --route-rules "[{\"cidrBlock\":\"0.0.0.0/0\",\"networkEntityId\":\"$IGW\"}]" >/dev/null

echo "== security list 22 + 4433 udp/tcp =="
SL=$(oci network security-list list --compartment-id "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[0].id' --raw-output)
oci network security-list update --security-list-id "$SL" --force \
  --egress-security-rules '[{"destination":"0.0.0.0/0","protocol":"all","isStateless":false}]' \
  --ingress-security-rules '[
    {"source":"0.0.0.0/0","protocol":"6","isStateless":false,"tcpOptions":{"destinationPortRange":{"min":22,"max":22}}},
    {"source":"0.0.0.0/0","protocol":"6","isStateless":false,"tcpOptions":{"destinationPortRange":{"min":4433,"max":4433}}},
    {"source":"0.0.0.0/0","protocol":"17","isStateless":false,"udpOptions":{"destinationPortRange":{"min":4433,"max":4433}}},
    {"source":"0.0.0.0/0","protocol":"1","isStateless":false,"icmpOptions":{"type":3,"code":4}}
  ]' >/dev/null

echo "== subnet =="
SUB=$(oci network subnet list --compartment-id "$COMPARTMENT" --vcn-id "$VCN" \
  --query 'data[0].id' --raw-output 2>/dev/null || true)
if [ -z "$SUB" ] || [ "$SUB" = "null" ]; then
  SUB=$(oci network subnet create --compartment-id "$COMPARTMENT" --vcn-id "$VCN" \
    --cidr-block 10.0.0.0/24 --display-name starmesh-public \
    --prohibit-public-ip-on-vnic false --wait-for-state AVAILABLE \
    --query 'data.id' --raw-output)
fi
echo "SUB=$SUB"

echo "== launch $NAME ($SHAPE) =="
INST=$(oci compute instance launch \
  --compartment-id "$COMPARTMENT" \
  --availability-domain "$AD" \
  --display-name "$NAME" \
  --shape "$SHAPE" \
  --image-id "$IMAGE" \
  --subnet-id "$SUB" \
  --assign-public-ip true \
  --ssh-authorized-keys-file "$SSH_KEY" \
  --user-data-file "$CLOUDINIT" \
  --wait-for-state RUNNING \
  --query 'data.id' --raw-output)
echo "INSTANCE=$INST"

echo "== public IP =="
VNIC=$(oci compute instance list-vnics --instance-id "$INST" --query 'data[0].id' --raw-output)
IP=$(oci network vnic get --vnic-id "$VNIC" --query 'data."public-ip"' --raw-output)
echo "PUBLIC_IP=$IP"
echo "$INST" > /tmp/starmesh-oci-instance.id
echo "$IP" > /tmp/starmesh-oci-instance.ip
echo "ssh -i ${SSH_KEY%.pub} ubuntu@$IP"

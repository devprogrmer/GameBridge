#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

REPO="https://github.com/devprogrmer/GameBridge"

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run as root: sudo bash install.sh"
  exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
  echo "This installer currently supports Debian/Ubuntu."
  exit 1
fi

echo "[1/6] Installing packages..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y \
  ca-certificates curl git golang-go build-essential \
  python3 iproute2 iptables nftables procps

echo "[2/6] Building GameBridge core..."
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SRC_DIR"
go mod download
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=0.1.0" \
  -o /usr/local/bin/gamebridge-core ./cmd/gamebridge-core

echo "[3/6] Installing manager..."
install -m 0755 scripts/gamebridge.py /usr/local/bin/gamebridge
mkdir -p /usr/local/lib/gamebridge /etc/gamebridge/kernel

echo "[4/6] Installing systemd units..."
install -m 0644 systemd/gamebridge@.service /etc/systemd/system/gamebridge@.service
install -m 0644 systemd/gamebridge-kernel@.service /etc/systemd/system/gamebridge-kernel@.service
install -m 0644 systemd/gamebridge-forward.service /etc/systemd/system/gamebridge-forward.service
install -m 0755 uninstall.sh /usr/local/lib/gamebridge/uninstall.sh

cat >/etc/sysctl.d/99-gamebridge-base.conf <<'EOF'
net.ipv4.ip_forward=1
EOF
sysctl --system >/dev/null || true

systemctl daemon-reload

echo "[5/6] Checking TUN..."
modprobe tun 2>/dev/null || true
if [[ ! -c /dev/net/tun ]]; then
  echo "WARNING: /dev/net/tun was not found. Ask your VPS provider to enable TUN/TAP."
fi

echo "[6/6] Done."
echo
echo "GameBridge installed."
echo "Run:"
echo "  gamebridge"
echo
echo "Documentation:"
echo "  $REPO"

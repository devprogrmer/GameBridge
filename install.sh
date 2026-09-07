#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

REPO_SLUG="devprogrmer/GameBridge"
REPO_URL="https://github.com/${REPO_SLUG}"
RAW_URL="https://raw.githubusercontent.com/${REPO_SLUG}"

log() {
  printf '[GameBridge] %s\n' "$*"
}

die() {
  printf '[GameBridge] ERROR: %s\n' "$*" >&2
  exit 1
}

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  die "Run as root: sudo bash install.sh"
fi

if ! command -v apt-get >/dev/null 2>&1; then
  die "This installer currently supports Debian/Ubuntu."
fi

case "$(uname -m)" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    die "Unsupported CPU architecture: $(uname -m). Supported: amd64, arm64."
    ;;
esac

log "Installing runtime dependencies..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y \
  ca-certificates curl coreutils kmod python3 \
  iproute2 iptables nftables procps

VERSION="${GAMEBRIDGE_VERSION:-}"

if [[ -z "$VERSION" ]]; then
  log "Resolving latest stable release..."
  latest_url="$(
    curl -fsSLI --retry 3 --connect-timeout 10 \
      -o /dev/null -w '%{url_effective}' \
      "${REPO_URL}/releases/latest"
  )" || die "Could not resolve latest release."

  VERSION="${latest_url##*/}"
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v${VERSION}"
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][A-Za-z0-9._-]+)?$ ]]; then
  die "Invalid release version: ${VERSION}"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

BINARY="gamebridge-core-linux-${ARCH}"
CHECKSUM="${BINARY}.sha256"
DOWNLOAD_BASE="${REPO_URL}/releases/download/${VERSION}"

log "Downloading ${BINARY} from ${VERSION}..."

curl -fL --retry 3 --connect-timeout 10 \
  -o "${TMP_DIR}/${BINARY}" \
  "${DOWNLOAD_BASE}/${BINARY}"

curl -fL --retry 3 --connect-timeout 10 \
  -o "${TMP_DIR}/${CHECKSUM}" \
  "${DOWNLOAD_BASE}/${CHECKSUM}"

log "Verifying SHA-256..."
(
  cd "$TMP_DIR"
  sha256sum -c "$CHECKSUM"
)

install -m 0755 "${TMP_DIR}/${BINARY}" /usr/local/bin/gamebridge-core

log "Installing manager and systemd services..."

SUPPORT_BASE="${RAW_URL}/${VERSION}"

curl -fL --retry 3 -o "${TMP_DIR}/gamebridge.py" \
  "${SUPPORT_BASE}/scripts/gamebridge.py"

curl -fL --retry 3 -o "${TMP_DIR}/gamebridge@.service" \
  "${SUPPORT_BASE}/systemd/gamebridge@.service"

curl -fL --retry 3 -o "${TMP_DIR}/gamebridge-kernel@.service" \
  "${SUPPORT_BASE}/systemd/gamebridge-kernel@.service"

curl -fL --retry 3 -o "${TMP_DIR}/gamebridge-forward.service" \
  "${SUPPORT_BASE}/systemd/gamebridge-forward.service"

curl -fL --retry 3 -o "${TMP_DIR}/uninstall.sh" \
  "${SUPPORT_BASE}/uninstall.sh"

install -m 0755 "${TMP_DIR}/gamebridge.py" /usr/local/bin/gamebridge

mkdir -p /usr/local/lib/gamebridge
mkdir -p /etc/gamebridge/kernel

install -m 0644 "${TMP_DIR}/gamebridge@.service" \
  /etc/systemd/system/gamebridge@.service

install -m 0644 "${TMP_DIR}/gamebridge-kernel@.service" \
  /etc/systemd/system/gamebridge-kernel@.service

install -m 0644 "${TMP_DIR}/gamebridge-forward.service" \
  /etc/systemd/system/gamebridge-forward.service

install -m 0755 "${TMP_DIR}/uninstall.sh" \
  /usr/local/lib/gamebridge/uninstall.sh

printf '%s\n' "$VERSION" >/etc/gamebridge/VERSION

cat >/etc/sysctl.d/99-gamebridge-base.conf <<'EOF'
net.ipv4.ip_forward=1
EOF

sysctl --system >/dev/null || true
systemctl daemon-reload

log "Checking TUN/TAP..."
modprobe tun 2>/dev/null || true

if [[ ! -c /dev/net/tun ]]; then
  printf '[GameBridge] WARNING: /dev/net/tun was not found. Enable TUN/TAP on the VPS.\n' >&2
fi

installed_version="$(/usr/local/bin/gamebridge-core version 2>/dev/null || true)"
log "Installed ${installed_version:-GameBridge ${VERSION}}"

printf '\nGameBridge is ready.\nRun:\n  sudo gamebridge\n\nRepository:\n  %s\n' "$REPO_URL"

#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail
REPO="https://github.com/devprogrmer/GameBridge"; RAW="https://raw.githubusercontent.com/devprogrmer/GameBridge"
die(){ echo "[GameBridge] ERROR: $*" >&2; exit 1; }
[[ ${EUID:-$(id -u)} -eq 0 ]] || die "Run as root."
case "$(uname -m)" in x86_64|amd64) ARCH=amd64;; aarch64|arm64) ARCH=arm64;; *) die "Unsupported architecture";; esac
command -v apt-get >/dev/null 2>&1 || die "Debian/Ubuntu only for now."
MODE="${GAMEBRIDGE_INSTALL_MODE:-all}"
[[ "$MODE" == "all" || "$MODE" == "panel" || "$MODE" == "agent" ]] || die "GAMEBRIDGE_INSTALL_MODE must be all, panel or agent"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl coreutils openssl wireguard-tools iproute2 iptables nftables procps kmod
VERSION="${GAMEBRIDGE_VERSION:-}"; if [[ -z "$VERSION" ]]; then VERSION="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${REPO}/releases/latest" | awk -F/ '{print $NF}')"; fi; [[ "$VERSION" == v* ]] || VERSION="v${VERSION}"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
install_component(){ local component="$1"; local bin="gamebridge-${component}-linux-${ARCH}"; curl -fL --retry 3 -o "$TMP/$bin" "${REPO}/releases/download/${VERSION}/${bin}"; curl -fL --retry 3 -o "$TMP/$bin.sha256" "${REPO}/releases/download/${VERSION}/${bin}.sha256"; (cd "$TMP" && sha256sum -c "$bin.sha256"); install -m 0755 "$TMP/$bin" "/usr/local/bin/gamebridge-${component}"; }
# Agents manage the existing GameBridge core. Install the matching core automatically if absent.
if [[ "$MODE" == "all" || "$MODE" == "agent" ]]; then
  if [[ ! -x /usr/local/bin/gamebridge-core ]]; then
    curl -fsSL "${RAW}/${VERSION}/install.sh" | GAMEBRIDGE_VERSION="$VERSION" bash
  fi
fi
mkdir -p /etc/gamebridge /var/lib/gamebridge; chmod 700 /etc/gamebridge /var/lib/gamebridge
if [[ "$MODE" == "all" || "$MODE" == "panel" ]]; then
  install_component panel
  curl -fL --retry 3 -o /etc/systemd/system/gamebridge-panel.service "${RAW}/${VERSION}/systemd/gamebridge-panel.service"
  if [[ ! -f /etc/gamebridge/panel.env ]]; then cat >/etc/gamebridge/panel.env <<'EOT'
GAMEBRIDGE_PANEL_LISTEN=0.0.0.0:8088
GAMEBRIDGE_PANEL_STATE=/var/lib/gamebridge/panel-state.json
GAMEBRIDGE_PANEL_SESSION_KEY=/etc/gamebridge/panel-session.key
GAMEBRIDGE_PANEL_MASTER_KEY=/etc/gamebridge/panel-master.key
GAMEBRIDGE_COOKIE_SECURE=0
EOT
  chmod 600 /etc/gamebridge/panel.env; fi
fi
if [[ "$MODE" == "all" || "$MODE" == "agent" ]]; then
  install_component agent
  curl -fL --retry 3 -o /etc/systemd/system/gamebridge-agent.service "${RAW}/${VERSION}/systemd/gamebridge-agent.service"
  if [[ ! -f /etc/gamebridge/agent.env ]]; then TOKEN="$(openssl rand -hex 32)"; cat >/etc/gamebridge/agent.env <<EOT
GAMEBRIDGE_AGENT_LISTEN=0.0.0.0:8089
GAMEBRIDGE_AGENT_TOKEN=${TOKEN}
EOT
  chmod 600 /etc/gamebridge/agent.env; fi
fi
systemctl daemon-reload
if [[ "$MODE" == "all" || "$MODE" == "panel" ]]; then systemctl enable --now gamebridge-panel.service; fi
if [[ "$MODE" == "all" || "$MODE" == "agent" ]]; then systemctl enable --now gamebridge-agent.service; fi
IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
if [[ "$MODE" == "all" || "$MODE" == "panel" ]]; then echo "Panel: http://${IP:-SERVER_IP}:8088"; fi
if [[ "$MODE" == "all" || "$MODE" == "agent" ]]; then echo "Agent: http://${IP:-SERVER_IP}:8089"; echo "Agent token:"; grep '^GAMEBRIDGE_AGENT_TOKEN=' /etc/gamebridge/agent.env | cut -d= -f2-; fi

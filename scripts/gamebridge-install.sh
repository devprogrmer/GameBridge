#!/usr/bin/env bash

# SPDX-License-Identifier: AGPL-3.0-only

set -euo pipefail

IMAGE="ghcr.io/devprogrmer/GameBridge:latest"
INSTALL_DIR="/opt/gamebridge"

log() {
    printf '[GameBridge] %s\n' "$*"
}

die() {
    printf '[GameBridge] ERROR: %s\n' "$*" >&2
    exit 1
}

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    die "Run as root: sudo bash gamebridge-install.sh"
fi

if ! command -v docker >/dev/null 2>&1; then
    die "Docker is required. Install Docker first."
fi

if ! docker compose version >/dev/null 2>&1; then
    die "Docker Compose plugin is required."
fi

log "Creating installation directory..."

mkdir -p "$INSTALL_DIR"

cd "$INSTALL_DIR"

log "Downloading production compose..."

curl -fsSL \
"https://raw.githubusercontent.com/devprogrmer/GameBridge/main/docker-compose.yml" \
-o docker-compose.yml

log "Pulling GameBridge image..."

docker compose pull

log "Starting GameBridge..."

docker compose up -d

log "Waiting for health check..."

for i in $(seq 1 30); do
    if curl -fsS http://127.0.0.1:8088/healthz >/dev/null; then
        echo
        echo "GameBridge is running."
        echo
        echo "Panel:"
        echo "http://SERVER_IP:8088"
        exit 0
    fi

    sleep 2
done

docker compose logs

die "GameBridge failed health check."
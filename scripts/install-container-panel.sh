#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

log() { printf '[GameBridge] %s\n' "$*"; }
die() { printf '[GameBridge] ERROR: %s\n' "$*" >&2; exit 1; }

[[ ${EUID:-$(id -u)} -eq 0 ]] || die "Run as root."

command -v docker >/dev/null 2>&1 || die "Docker is required. Install Docker Engine first."
docker info >/dev/null 2>&1 || die "Docker daemon is not available."

PORT="${GAMEBRIDGE_PANEL_PORT:-8088}"
BIND="${GAMEBRIDGE_PANEL_BIND:-0.0.0.0}"
COOKIE_SECURE="${GAMEBRIDGE_COOKIE_SECURE:-0}"
VERSION="${GAMEBRIDGE_VERSION:-latest}"
IMAGE="${GAMEBRIDGE_IMAGE:-ghcr.io/devprogrmer/gamebridge:${VERSION}}"
CONTAINER="gamebridge-panel"
VOLUME="gamebridge-data"

case "$PORT" in
  ''|*[!0-9]*) die "GAMEBRIDGE_PANEL_PORT must be numeric." ;;
esac
(( PORT >= 1 && PORT <= 65535 )) || die "GAMEBRIDGE_PANEL_PORT must be between 1 and 65535."
[[ "$COOKIE_SECURE" == "0" || "$COOKIE_SECURE" == "1" ]] || die "GAMEBRIDGE_COOKIE_SECURE must be 0 or 1."

if docker container inspect "$CONTAINER" >/dev/null 2>&1; then
  managed="$(docker inspect --format '{{ index .Config.Labels "io.gamebridge.managed" }}' "$CONTAINER" 2>/dev/null || true)"
  [[ "$managed" == "1" ]] || die "Container ${CONTAINER} already exists but is not managed by this installer."
fi

log "Pulling ${IMAGE}..."
docker pull "$IMAGE"

docker volume create "$VOLUME" >/dev/null

if docker container inspect "$CONTAINER" >/dev/null 2>&1; then
  log "Replacing the managed panel container..."
  docker rm -f "$CONTAINER" >/dev/null
fi

log "Starting GameBridge Panel..."
docker run -d \
  --name "$CONTAINER" \
  --restart unless-stopped \
  --init \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  --label io.gamebridge.managed=1 \
  -p "${BIND}:${PORT}:8088" \
  -e "GAMEBRIDGE_COOKIE_SECURE=${COOKIE_SECURE}" \
  -v "${VOLUME}:/data" \
  "$IMAGE" >/dev/null

log "Waiting for health check..."
for _ in $(seq 1 45); do
  health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}starting{{end}}' "$CONTAINER" 2>/dev/null || true)"
  case "$health" in
    healthy)
      IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
      printf '\nGameBridge Panel is ready.\n'
      printf 'URL: http://%s:%s\n' "${IP:-SERVER_IP}" "$PORT"
      printf 'Container: %s\n' "$CONTAINER"
      printf 'Persistent volume: %s\n' "$VOLUME"
      printf '\nCreate the Owner account in the browser on first launch.\n'
      exit 0
      ;;
    unhealthy)
      docker logs --tail 80 "$CONTAINER" >&2 || true
      die "Container health check failed."
      ;;
  esac
  sleep 2
done

docker logs --tail 80 "$CONTAINER" >&2 || true
die "Timed out waiting for GameBridge Panel health."
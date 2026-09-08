# GameBridge Production Deployment

GameBridge uses a split production model:

- **Panel / Control Center:** containerized and published as a multi-architecture image.
- **Node Agent:** installed natively on each Linux node because it manages host networking, WireGuard, OpenVPN, Tor, Xray, routes, interfaces, and systemd services.

Running the node agent in a privileged container is intentionally not the default deployment model.

## Quick start: Panel with Docker

The stable image is:

```text
ghcr.io/devprogrmer/gamebridge:latest
```

Start the panel with one Docker command:

```bash
docker run -d \
  --name gamebridge-panel \
  --restart unless-stopped \
  --init \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --security-opt no-new-privileges \
  --cap-drop ALL \
  -p 8088:8088 \
  -v gamebridge-data:/data \
  ghcr.io/devprogrmer/gamebridge:latest
```

Open:

```text
http://SERVER_IP:8088
```

On first launch, create the Owner account.

The named `gamebridge-data` volume stores:

- panel state
- session signing key
- master encryption key

Do not delete this volume during upgrades.

## One-line container installer

If Docker Engine is already installed:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/scripts/install-container-panel.sh | sudo bash
```

Optional environment variables:

```bash
GAMEBRIDGE_PANEL_PORT=8088
GAMEBRIDGE_PANEL_BIND=0.0.0.0
GAMEBRIDGE_COOKIE_SECURE=0
GAMEBRIDGE_VERSION=latest
GAMEBRIDGE_IMAGE=ghcr.io/devprogrmer/gamebridge:latest
```

For an HTTPS reverse proxy, set:

```bash
GAMEBRIDGE_COOKIE_SECURE=1
```

## Docker Compose

```bash
curl -fsSLO https://raw.githubusercontent.com/devprogrmer/GameBridge/main/docker-compose.yml
docker compose up -d
```

Override settings with environment variables before running Compose.

## Install a native GameBridge Node

On every server that will carry tunnels, inbounds, outbounds, WireGuard, OpenVPN, Tor, or Xray:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/install-node.sh | sudo bash
```

The installer prints the node agent token. It is also stored in:

```text
/etc/gamebridge/agent.env
```

Add the node in the GameBridge Control Center with:

```text
Agent URL: http://NODE_IP:8089
Agent Token: value of GAMEBRIDGE_AGENT_TOKEN
```

Protect port `8089` with a firewall and allow it only from the GameBridge panel/control network whenever possible.

## Health check

The container exposes an unauthenticated liveness endpoint:

```text
GET /healthz
```

It returns HTTP 200 when the panel process is healthy.

Docker also includes a built-in `HEALTHCHECK`.

## Upgrade the Panel

With the one-line installer:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/scripts/install-container-panel.sh | sudo bash
```

With Compose:

```bash
docker compose pull
docker compose up -d
```

The persistent volume is reused automatically.

## Backup

Example backup of the Docker named volume:

```bash
docker run --rm \
  -v gamebridge-data:/data:ro \
  -v "$PWD":/backup \
  alpine:3.22 \
  tar czf /backup/gamebridge-data-backup.tar.gz -C /data .
```

Store backups securely because the volume contains the panel master encryption key.

## Restore

Stop the panel first, restore the volume contents, then start the container again.

## Release artifacts

Every stable `v*` tag publishes:

- `gamebridge-core-linux-amd64`
- `gamebridge-core-linux-arm64`
- `gamebridge-panel-linux-amd64`
- `gamebridge-panel-linux-arm64`
- `gamebridge-agent-linux-amd64`
- `gamebridge-agent-linux-arm64`
- SHA-256 checksums
- multi-architecture Panel image for `linux/amd64` and `linux/arm64`

Container registry:

```text
ghcr.io/devprogrmer/gamebridge
```

Stable releases receive the `latest` tag in addition to version tags.
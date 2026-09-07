# GameBridge Phase 3 — Routing & Unified Accounting

Phase 3 expands GameBridge from operational Xray inbounds into a real
routing/accounting control plane while keeping **GameBridge Tunnels** as a
separate Infrastructure feature.

## Added

### Outbounds

Panel CRUD and Xray deployment for:

- Freedom
- Blackhole
- SOCKS
- HTTP

Custom proxy passwords are encrypted at rest.

API:

```text
GET    /api/outbounds
POST   /api/outbounds
GET    /api/outbounds/{id}
PUT    /api/outbounds/{id}
DELETE /api/outbounds/{id}
```

### Routing

Rules can match:

- inbound
- user
- domain
- IP / CIDR / geoip
- port / port range
- network
- sniffed protocol

and route to:

- direct
- blocked
- a custom outbound

Rules have explicit priority and are deployed into each node's Xray config.

API:

```text
GET    /api/routing
POST   /api/routing
GET    /api/routing/{id}
PUT    /api/routing/{id}
DELETE /api/routing/{id}
```

### Unified traffic accounting

GameBridge now combines:

```text
Xray traffic + WireGuard traffic = User traffic
```

Xray user counters are collected through the local Xray Stats API on each
node. Stats API is bound to `127.0.0.1:10085`, not a public interface.

The panel periodically:

1. reads WireGuard counters,
2. reads and resets Xray user counters,
3. updates per-user totals,
4. processes periodic traffic resets,
5. enforces expiry and quota,
6. disables or re-enables Xray users and WireGuard peers.

### Online users

Xray activity and recent WireGuard handshakes/traffic are used as activity
signals. This phase intentionally does not claim a precise unique-IP count.

API:

```text
GET /api/online-users
GET /api/traffic
POST /api/sync-traffic
```

### Periodic reset

Plans now support:

```text
reset_interval_days
```

Users can also have an override at the model/API level. When a reset occurs:

- Xray usage resets
- WireGuard accounting baselines reset
- total usage resets
- quota-exceeded users become active again
- Xray clients/WireGuard peers are restored

### WireGuard address allocation fix

Client addresses are allocated across all peers on the same node/interface,
not per user. For a server address such as `10.77.0.1/24`, the first automatic
client address is `10.77.0.2/32`.

Custom client addresses are validated against the subnet, reserved addresses,
and existing peers.

## Critical state-store hardening

Earlier beta state cloning/persistence used JSON on public models whose secret
fields were marked `json:"-"`. That could strip encrypted tokens/credentials
from read clones and from persisted state.

Phase 3 adds a private disk representation and regression tests so these stay
persistent while remaining hidden from normal API JSON:

- admin password hashes and TOTP secrets
- node Agent tokens
- REALITY private keys
- Shadowsocks server secrets
- per-user Xray credentials
- outbound proxy passwords
- WireGuard private keys/configs

## UI

New pages:

```text
Outbounds
Routing
Online Users
Traffic
```

The Plan form also includes traffic reset interval.

## Important upgrade note

A secret that was already lost by an older beta state file cannot be recovered
mathematically by this update. Phase 3 prevents future loss. If an old deployed
beta instance was restarted after saving a state file with missing secrets,
re-provision the affected admin/node/Xray/WireGuard secret material before
production use.

## Still planned

The next panel phases can add:

- WireGuard/WARP/Sing-box outbounds
- richer route groups/policies
- native Xray connection/session telemetry
- Clash / Sing-box / V2Ray subscription formats
- QR codes
- certificate automation
- backup/restore
- reseller scopes

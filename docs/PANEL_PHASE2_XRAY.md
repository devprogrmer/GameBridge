# Phase 2 — Xray Inbounds Runtime

Phase 2 makes `Inbounds` an operational VPN/proxy feature and keeps
`GameBridge Tunnels` isolated under Infrastructure.

## Implemented

- Xray detection on every GameBridge node
- Xray install action through the authenticated node agent
- Atomic Xray config apply
- `xray run -test` validation before restart
- Automatic rollback to the previous config if restart fails
- VLESS, VMess, Trojan and Shadowsocks inbound models
- TCP, WebSocket and gRPC transports
- TLS certificate/key paths
- VLESS/Trojan REALITY (TCP in this phase)
- X25519 key generation on the target node
- REALITY private key encryption-at-rest in the panel state
- Users attached to inbounds with encrypted per-user credentials
- VLESS/VMess/Trojan/Shadowsocks share links
- Subscription output now contains Xray links plus existing WireGuard configs
- Web UI for Inbounds, Xray status/install, Deploy and Users
- Tunnel feature remains independent under `/api/tunnels`

## Agent API

```text
GET  /v1/xray/status
POST /v1/xray/install
POST /v1/xray/x25519
POST /v1/xray/apply
```

The agent validates a candidate config before replacing:

```text
/usr/local/etc/xray/config.json
```

## Panel API

```text
GET    /api/inbounds
POST   /api/inbounds
GET    /api/inbounds/{id}
PUT    /api/inbounds/{id}
DELETE /api/inbounds/{id}

POST   /api/inbounds/{id}/deploy
GET    /api/inbounds/{id}/xray-status
POST   /api/inbounds/{id}/xray-install

POST   /api/inbounds/{id}/users
DELETE /api/inbounds/{id}/users/{user_id}
GET    /api/inbounds/{id}/users/{user_id}/share
```

## TLS

For `tls`, the certificate and private-key paths must already exist on the
target node. Certificate automation is a later phase.

## REALITY

Phase 2 supports REALITY for VLESS/Trojan over TCP. The target node generates
the X25519 key pair. Only the public key is exposed by the API; the private
key is encrypted in panel state.

## Important

This is still a development branch. Test on disposable VPS nodes before
using it for production traffic.

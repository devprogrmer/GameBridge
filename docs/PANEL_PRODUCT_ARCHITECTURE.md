# GameBridge Panel-first architecture

GameBridge is a VPN / proxy management platform. Tunneling is a separate
infrastructure feature inside the platform.

## Product areas

### Access
- Users
- Groups
- Plans
- Subscriptions
- Online users
- Device limits
- Traffic / expiry lifecycle

### Proxy & VPN
- Inbounds
- Outbounds
- Routing
- WireGuard
- Hysteria2
- Xray settings

### Infrastructure
- Nodes
- GameBridge Tunnels
- Port Forward
- DNS
- Firewall
- Monitoring

### Management
- Admins / RBAC
- Resellers
- Telegram / Webhooks
- Backup / Restore
- Audit
- Updates
- Settings

## Tunnel boundary

Tunnel APIs stay under:

```text
/api/tunnels
```

and are treated as an infrastructure subsystem.

Proxy/VPN ingress management uses:

```text
/api/inbounds
```

The two domains must not share UI forms or lifecycle logic.

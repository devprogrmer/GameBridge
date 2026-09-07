# Tunnels feature

`GameBridge Tunnels` is an infrastructure feature, not the main product.

This module owns:

- PulseUDP
- PulseUDP MultiPath
- KCP
- QUIC
- ICMP
- GRE / GRE6
- IPIP / SIT
- Geneve / VXLAN
- tunnel health / benchmark / MTU / path metrics

The main panel owns users, inbounds, outbounds, subscriptions, accounting,
nodes, administration, monitoring and system settings.

The next refactor moves the current tunnel handlers out of
`internal/panel/operations.go` into this feature boundary without changing
the public `/api/tunnels` contract.

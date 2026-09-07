# GameBridge Architecture

GameBridge has two classes of transports.

## User-space transports

`gamebridge-core` creates a Linux TUN interface and forwards IP frames through:
- PulseUDP
- PulseUDP MultiPath
- SwiftKCP
- GameQUIC
- EchoGame / ICMP

## Kernel transports

The Python manager creates and persists:
- GRE
- GRE6
- IPIP
- SIT / 6in4
- Geneve
- VXLAN

## Control plane

The `gamebridge` Python CLI:
- creates configs
- generates keys
- installs systemd services
- configures NAT
- creates port-forward rules
- exposes status, logs and benchmark commands

#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
import json
import os
import re
import secrets
import shlex
import subprocess
import sys
from pathlib import Path

ETC = Path("/etc/gamebridge")
KERNEL = ETC / "kernel"
FORWARD = ETC / "forward-rules.sh"

def c(text, code):
    return f"\033[{code}m{text}\033[0m"

def title():
    print(c("╔══════════════════════════════════════════════╗", "36"))
    print(c("║              GameBridge                     ║", "36"))
    print(c("║     Multi-Transport Gaming Tunnel            ║", "36"))
    print(c("╚══════════════════════════════════════════════╝", "36"))

def need_root():
    if os.geteuid() != 0:
        print("Run with sudo/root.")
        sys.exit(1)

def run(*args, check=True, capture=False):
    if capture:
        return subprocess.run(args, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check).stdout
    return subprocess.run(args, check=check).returncode

def ask(label, default=None):
    suffix = f" [{default}]" if default is not None else ""
    v = input(f"{label}{suffix}: ").strip()
    return v if v else (str(default) if default is not None else "")

def yesno(label, default=True):
    d = "Y/n" if default else "y/N"
    v = input(f"{label} [{d}]: ").strip().lower()
    if not v:
        return default
    return v in ("y", "yes", "1")

def choose(label, options, default=1):
    print()
    print(label)
    for i, x in enumerate(options, 1):
        print(f"  {i}) {x}")
    while True:
        v = ask("Select", default)
        try:
            n = int(v)
            if 1 <= n <= len(options):
                return n
        except ValueError:
            pass
        print("Invalid choice.")

def safe_name(s):
    if not re.fullmatch(r"[A-Za-z0-9_.-]{1,40}", s):
        raise ValueError("Name may only contain letters, numbers, dot, underscore and dash.")
    return s

def default_iface():
    out = run("ip", "route", "show", "default", capture=True, check=False)
    m = re.search(r"\bdev\s+(\S+)", out)
    return m.group(1) if m else "eth0"

def write_json(name, data):
    ETC.mkdir(parents=True, exist_ok=True)
    p = ETC / f"{name}.json"
    p.write_text(json.dumps(data, indent=2) + "\n")
    os.chmod(p, 0o600)
    return p

def enable_user_service(name):
    run("systemctl", "daemon-reload")
    run("systemctl", "enable", "--now", f"gamebridge@{name}.service")

def common_userspace(transport):
    need_root()
    title()
    print(f"\nConfigure: {transport}\n")
    name = safe_name(ask("Tunnel name", transport))
    role_i = choose("This server is:", ["Iran / Client", "Kharej / Server"])
    role = "client" if role_i == 1 else "server"

    peer_host = ""
    listen_host = "0.0.0.0"
    if role == "client":
        peer_host = ask("Kharej public IPv4/IPv6")
    ports = []
    if transport != "icmp":
        if transport == "pulseudp-mp":
            raw = ask("UDP ports (comma separated)", "9000,9001,9002,9003")
        else:
            raw = ask("Tunnel UDP port", {"pulseudp":"9000","kcp":"8443","quic":"8443"}.get(transport,"9000"))
        ports = [int(x.strip()) for x in raw.split(",") if x.strip()]

    iface = ask("Tunnel interface", "gb0")
    if role == "client":
        local_cidr = ask("Iran tunnel IP", "10.20.0.1/30")
        peer_ip = ask("Kharej tunnel IP (without /30)", "10.20.0.2")
    else:
        local_cidr = ask("Kharej tunnel IP", "10.20.0.2/30")
        peer_ip = ask("Iran tunnel IP (without /30)", "10.20.0.1")

    mtu_defaults = {"icmp":900, "quic":1280}
    mtu = int(ask("MTU", mtu_defaults.get(transport, 1360)))

    profiles = ["competitive", "balanced", "stable", "lossy"]
    pi = choose("Gaming profile:", ["Competitive - lowest latency", "Balanced", "Stable", "Lossy - more recovery"])
    profile = profiles[pi-1]

    print("\nBoth servers MUST use the same 64-character key.")
    if role == "client" and yesno("Generate a new key now", True):
        key = secrets.token_hex(32)
        print(c("\nCOPY THIS KEY TO THE KHAREJ SERVER:", "33"))
        print(c(key, "32"))
        print()
    else:
        key = ask("Paste the same 64-character key")
    if not re.fullmatch(r"[0-9a-fA-F]{64}", key):
        raise ValueError("Key must be exactly 64 hexadecimal characters.")

    data = {
        "name": name,
        "role": role,
        "transport": transport,
        "listen_host": listen_host,
        "peer_host": peer_host,
        "ports": ports,
        "interface": iface,
        "local_cidr": local_cidr,
        "peer_ip": peer_ip,
        "mtu": mtu,
        "key_hex": key.lower(),
        "profile": profile,
        "duplicate_small_packets": profile in ("stable", "lossy") and transport == "pulseudp-mp",
        "duplicate_threshold": 512,
        "kcp_data_shards": 10 if transport == "kcp" else 0,
        "kcp_parity_shards": (3 if profile == "lossy" else 2 if profile == "stable" else 0) if transport == "kcp" else 0,
        "icmp_id": int(ask("ICMP tunnel ID", "4242")) if transport == "icmp" else 0,
        "icmp_poll_ms": int(ask("ICMP poll ms", "8")) if transport == "icmp" else 0,
        "icmp_burst": int(ask("ICMP burst", "4")) if transport == "icmp" else 0,
        "nat": False,
        "internet_interface": "",
        "keepalive_ms": 1000,
        "path_timeout_ms": 5000,
    }
    if role == "server":
        data["nat"] = yesno("Enable Internet NAT on Kharej", True)
        if data["nat"]:
            data["internet_interface"] = ask("Internet interface", default_iface())

    p = write_json(name, data)
    enable_user_service(name)
    print(c(f"\n✓ Saved: {p}", "32"))
    print(c(f"✓ Started: gamebridge@{name}", "32"))
    print(f"\nTest peer tunnel IP:\n  ping {peer_ip}")
    print(f"Status:\n  gamebridge status {name}")
    print(f"Logs:\n  journalctl -u gamebridge@{name} -f")

def cmdline(argv):
    return " ".join(shlex.quote(x) for x in argv)

def create_kernel_script(name, up_cmds, down_cmds):
    KERNEL.mkdir(parents=True, exist_ok=True)
    p = KERNEL / f"{name}.sh"
    body = ["#!/usr/bin/env bash", "set -e", 'case "${1:-up}" in', "up)"]
    for x in up_cmds:
        body.append("  " + cmdline(x))
    body += ["  ;;", "down)"]
    for x in down_cmds:
        body.append("  " + cmdline(x))
    body += ["  ;;", '*) echo "usage: $0 up|down"; exit 2 ;;', "esac", ""]
    p.write_text("\n".join(body))
    os.chmod(p, 0o755)
    run("systemctl", "daemon-reload")
    run("systemctl", "enable", "--now", f"gamebridge-kernel@{name}.service")
    print(c(f"\n✓ Kernel tunnel {name} started.", "32"))

def kernel_tunnel(kind):
    need_root()
    title()
    name = safe_name(ask("Tunnel name", "gb-"+kind))
    dev = ask("Tunnel interface", "gbk0")
    local_pub = ask("This server public IP")
    remote_pub = ask("Remote server public IP")

    if kind in ("gre", "ipip"):
        local_cidr = ask("This server private tunnel IP", "10.30.0.1/30")
        mtu = ask("MTU", "1400")
        up = [
            ["ip", "link", "del", dev],
            ["ip", "tunnel", "add", dev, "mode", kind, "local", local_pub, "remote", remote_pub, "ttl", "255"],
            ["ip", "addr", "add", local_cidr, "dev", dev],
            ["ip", "link", "set", dev, "mtu", mtu, "up"],
        ]
    elif kind == "sit":
        local_cidr = ask("This server private IPv6", "fd42:20::1/64")
        mtu = ask("MTU", "1400")
        up = [
            ["ip", "link", "del", dev],
            ["ip", "tunnel", "add", dev, "mode", "sit", "local", local_pub, "remote", remote_pub, "ttl", "255"],
            ["ip", "-6", "addr", "add", local_cidr, "dev", dev],
            ["ip", "link", "set", dev, "mtu", mtu, "up"],
        ]
    elif kind == "gre6":
        local_cidr = ask("This server private IPv4 tunnel IP", "10.31.0.1/30")
        mtu = ask("MTU", "1380")
        up = [
            ["ip", "link", "del", dev],
            ["ip", "-6", "tunnel", "add", dev, "mode", "ip6gre", "local", local_pub, "remote", remote_pub],
            ["ip", "addr", "add", local_cidr, "dev", dev],
            ["ip", "link", "set", dev, "mtu", mtu, "up"],
        ]
    elif kind in ("geneve", "vxlan"):
        local_cidr = ask("This server private tunnel IP", "10.40.0.1/30")
        vni = ask("VNI", "100")
        mtu = ask("MTU", "1350")
        if kind == "geneve":
            port = ask("Geneve UDP port", "6081")
            add = ["ip", "link", "add", dev, "type", "geneve", "id", vni, "remote", remote_pub, "dstport", port]
        else:
            port = ask("VXLAN UDP port", "4789")
            outif = ask("Internet interface", default_iface())
            add = ["ip", "link", "add", dev, "type", "vxlan", "id", vni, "remote", remote_pub, "local", local_pub, "dstport", port, "dev", outif]
        up = [
            ["ip", "link", "del", dev],
            add,
            ["ip", "addr", "add", local_cidr, "dev", dev],
            ["ip", "link", "set", dev, "mtu", mtu, "up"],
        ]
    else:
        raise ValueError("Unknown kernel tunnel")

    # The first "del" command is intentionally allowed to fail.
    # Convert it into a shell-safe command using `|| true` by using a small wrapper.
    # The generated runner starts with set -e, so replace with `sh -c`.
    up[0] = ["sh", "-c", f"ip link del {shlex.quote(dev)} 2>/dev/null || true"]
    down = [["sh", "-c", f"ip link del {shlex.quote(dev)} 2>/dev/null || true"]]
    create_kernel_script(name, up, down)
    print(f"Check:\n  ip addr show {dev}")

def add_forward_rule():
    need_root()
    title()
    proto_i = choose("Protocol:", ["UDP (WireGuard / games)", "TCP", "Both"])
    proto = ["udp", "tcp", "both"][proto_i-1]
    listen_port = int(ask("Port users connect to on Iran", "51820"))
    dst_ip = ask("Kharej tunnel/private IP", "10.20.0.2")
    dst_port = int(ask("Destination port on Kharej", str(listen_port)))

    ETC.mkdir(parents=True, exist_ok=True)
    if not FORWARD.exists():
        FORWARD.write_text("#!/usr/bin/env bash\nset -e\nsysctl -w net.ipv4.ip_forward=1 >/dev/null\n")
        os.chmod(FORWARD, 0o755)
    protocols = ["udp", "tcp"] if proto == "both" else [proto]
    with FORWARD.open("a") as f:
        for p in protocols:
            f.write(f"iptables -t nat -C PREROUTING -p {p} --dport {listen_port} -j DNAT --to-destination {dst_ip}:{dst_port} 2>/dev/null || "
                    f"iptables -t nat -A PREROUTING -p {p} --dport {listen_port} -j DNAT --to-destination {dst_ip}:{dst_port}\n")
            f.write(f"iptables -C FORWARD -p {p} -d {dst_ip} --dport {dst_port} -j ACCEPT 2>/dev/null || "
                    f"iptables -A FORWARD -p {p} -d {dst_ip} --dport {dst_port} -j ACCEPT\n")
            f.write(f"iptables -t nat -C POSTROUTING -p {p} -d {dst_ip} --dport {dst_port} -j MASQUERADE 2>/dev/null || "
                    f"iptables -t nat -A POSTROUTING -p {p} -d {dst_ip} --dport {dst_port} -j MASQUERADE\n")
    run("systemctl", "daemon-reload")
    run("systemctl", "enable", "--now", "gamebridge-forward.service")
    print(c("\n✓ Port forwarding installed.", "32"))
    print(f"Users can use IranIP:{listen_port}")

def optimize():
    need_root()
    iface = ask("Internet interface", default_iface())
    Path("/etc/sysctl.d/99-gamebridge.conf").write_text("""\
net.ipv4.ip_forward=1
net.core.rmem_max=16777216
net.core.wmem_max=16777216
net.core.netdev_max_backlog=8192
""")
    run("sysctl", "--system", check=False)
    run("tc", "qdisc", "replace", "dev", iface, "root", "fq_codel", check=False)
    print(c("✓ Conservative gaming network tuning applied.", "32"))

def status(name=None):
    need_root()
    if name:
        p = Path(f"/run/gamebridge-{name}.json")
        if p.exists():
            print(p.read_text())
        run("systemctl", "--no-pager", "--full", "status", f"gamebridge@{name}", check=False)
        return
    print("\nUser-space services:")
    run("systemctl", "--no-pager", "--plain", "list-units", "gamebridge@*.service", check=False)
    print("\nKernel services:")
    run("systemctl", "--no-pager", "--plain", "list-units", "gamebridge-kernel@*.service", check=False)
    print("\nInterfaces:")
    run("ip", "-br", "addr", check=False)

def benchmark(target=None):
    need_root()
    if not target:
        target = ask("Tunnel peer IP to test", "10.20.0.2")
    print(c(f"\nTesting {target} with 50 packets...\n", "36"))
    run("ping", "-c", "50", "-i", "0.1", target, check=False)

def menu():
    need_root()
    while True:
        title()
        print("""
  User-space gaming transports
    1) PulseUDP
    2) PulseUDP MultiPath
    3) SwiftKCP / KCP-FEC
    4) GameQUIC
    5) EchoGame / ICMP fallback

  Native Linux tunnels
    6) GRE
    7) GRE6
    8) IPIP
    9) SIT / 6in4
   10) Geneve
   11) VXLAN

  Gaming / WireGuard
   12) Port Forward / WireGuard Relay

  Tools
   13) Status
   14) Benchmark
   15) Optimize server
   16) Logs
   17) Restart a tunnel
   18) Uninstall
    0) Exit
""")
        x = ask("Choice", "0")
        try:
            if x == "1": common_userspace("pulseudp")
            elif x == "2": common_userspace("pulseudp-mp")
            elif x == "3": common_userspace("kcp")
            elif x == "4": common_userspace("quic")
            elif x == "5": common_userspace("icmp")
            elif x == "6": kernel_tunnel("gre")
            elif x == "7": kernel_tunnel("gre6")
            elif x == "8": kernel_tunnel("ipip")
            elif x == "9": kernel_tunnel("sit")
            elif x == "10": kernel_tunnel("geneve")
            elif x == "11": kernel_tunnel("vxlan")
            elif x == "12": add_forward_rule()
            elif x == "13": status()
            elif x == "14": benchmark()
            elif x == "15": optimize()
            elif x == "16":
                n = ask("Tunnel name")
                run("journalctl", "-u", f"gamebridge@{n}", "-f", check=False)
            elif x == "17":
                n = ask("Tunnel name")
                run("systemctl", "restart", f"gamebridge@{n}", check=False)
                run("systemctl", "restart", f"gamebridge-kernel@{n}", check=False)
            elif x == "18":
                run("/usr/local/lib/gamebridge/uninstall.sh", check=False)
                return
            elif x == "0": return
        except Exception as e:
            print(c(f"\nERROR: {e}", "31"))
        input("\nPress Enter to continue...")

def main():
    if len(sys.argv) == 1:
        menu()
        return
    cmd = sys.argv[1]
    if cmd == "menu": menu()
    elif cmd == "status": status(sys.argv[2] if len(sys.argv) > 2 else None)
    elif cmd == "benchmark": benchmark(sys.argv[2] if len(sys.argv) > 2 else None)
    elif cmd == "optimize": optimize()
    elif cmd == "keygen": print(secrets.token_hex(32))
    elif cmd == "version": run("/usr/local/bin/gamebridge-core", "version", check=False)
    elif cmd == "logs" and len(sys.argv) > 2: run("journalctl", "-u", f"gamebridge@{sys.argv[2]}", "-f", check=False)
    elif cmd == "restart" and len(sys.argv) > 2:
        run("systemctl", "restart", f"gamebridge@{sys.argv[2]}", check=False)
        run("systemctl", "restart", f"gamebridge-kernel@{sys.argv[2]}", check=False)
    else:
        print("Usage: gamebridge [menu|status [name]|benchmark [ip]|optimize|keygen|version|logs NAME|restart NAME]")
        sys.exit(2)

if __name__ == "__main__":
    main()

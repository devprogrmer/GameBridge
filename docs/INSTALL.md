# GameBridge Installation

## Quick install

روی Ubuntu یا Debian:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/install.sh | sudo bash
```

Installer به‌صورت خودکار:

- آخرین نسخه Stable را پیدا می‌کند.
- معماری CPU را تشخیص می‌دهد.
- باینری AMD64 یا ARM64 را دانلود می‌کند.
- SHA-256 باینری را بررسی می‌کند.
- Core و Manager را نصب می‌کند.
- سرویس‌های systemd را نصب می‌کند.
- IP forwarding را فعال می‌کند.
- وجود TUN/TAP را بررسی می‌کند.

## Install a specific version

مثلاً برای `v0.1.0`:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/install.sh | sudo env GAMEBRIDGE_VERSION=v0.1.0 bash
```

## Supported architectures

```text
x86_64 / amd64  -> gamebridge-core-linux-amd64
aarch64 / arm64 -> gamebridge-core-linux-arm64
```

## Check installed version

```bash
gamebridge-core version
```

یا:

```bash
cat /etc/gamebridge/VERSION
```

## Start

```bash
sudo gamebridge
```

## TUN/TAP

بررسی:

```bash
ls -l /dev/net/tun
```

اگر وجود نداشت، TUN/TAP باید از پنل VPS یا Provider فعال شود.

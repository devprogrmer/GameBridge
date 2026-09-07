# GameBridge Web Panel

این Patch یک Control Plane واقعی به GameBridge اضافه می‌کند؛ فقط UI نمایشی نیست.

## امکانات

- Dashboard با Node health، Tunnel count، Traffic و Audit
- First-run Owner setup و Login
- Roleها: Owner / Admin / Operator / Viewer
- User Management
- Plan Management: حجم، مدت، Device Limit
- Subscription token برای هر کاربر
- WireGuard provisioning، config generation و traffic sync
- قطع خودکار WireGuard peer در Expire یا عبور از quota
- Node Agent با Bearer Token
- Tunnel Wizard برای PulseUDP، PulseUDP-MP، KCP، QUIC، ICMP، GRE، GRE6، IPIP، SIT، Geneve و VXLAN
- Start / Stop / Restart / Delete از وب
- TCP / UDP / Both Port Forward
- Remote journalctl logs
- Audit Log
- Optional TOTP برای ادمین‌های جدید
- Encryption-at-rest برای Agent token و WireGuard config
- PBKDF2-SHA256 password hashing
- Signed session cookie + CSRF protection

## معماری

```text
Browser
  |
GameBridge Panel :8088
  |---- Iran Agent :8089 ---- gamebridge-core / systemd / wg / iptables
  |---- Kharej Agent :8089 -- gamebridge-core / systemd / wg / iptables
```

## نصب بعد از Release جدید

روی سرور Control Plane (Panel + Agent):

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/install-panel.sh | sudo bash
```

روی Nodeهای دیگر فقط Agent + Core:

```bash
curl -fsSL https://raw.githubusercontent.com/devprogrmer/GameBridge/main/install-panel.sh | sudo GAMEBRIDGE_INSTALL_MODE=agent bash
```

بعد روی سرور Panel:

```text
http://SERVER_IP:8088
```

در اولین ورود حساب Owner را می‌سازی.

## اضافه کردن Node

توکن Agent:

```bash
grep GAMEBRIDGE_AGENT_TOKEN /etc/gamebridge/agent.env
```

در پنل:

```text
Nodes -> New
Name: Iran-01
Role: Iran
Public IP: x.x.x.x
Agent URL: http://x.x.x.x:8089
Agent Token: ...
Internet Interface: eth0
```

پورت 8089 را فقط برای IP سرور Panel باز کن یا Agent را پشت HTTPS قرار بده.

## نکته امنیتی Production

- Panel را پشت Caddy/Nginx و HTTPS قرار بده.
- در `/etc/gamebridge/panel.env` مقدار `GAMEBRIDGE_COOKIE_SECURE=1` کن.
- `/etc/gamebridge/panel-master.key` و `/var/lib/gamebridge/panel-state.json` را Backup کن.
- Agent port را عمومی نگذار.

## Store

این Beta از State Store اتمیک و رمزگذاری‌شده در فایل استفاده می‌کند تا dependency جدید به پروژه اضافه نشود. برای نصب‌های بسیار بزرگ، PostgreSQL migration مرحله بعدی است.

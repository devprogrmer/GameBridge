#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -e

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "Run as root."
  exit 1
fi

echo "Stopping GameBridge services..."
systemctl list-unit-files --no-legend 'gamebridge@*.service' 2>/dev/null | awk '{print $1}' | while read -r s; do
  systemctl disable --now "$s" 2>/dev/null || true
done
systemctl list-unit-files --no-legend 'gamebridge-kernel@*.service' 2>/dev/null | awk '{print $1}' | while read -r s; do
  systemctl disable --now "$s" 2>/dev/null || true
done
systemctl disable --now gamebridge-forward.service 2>/dev/null || true

rm -f /usr/local/bin/gamebridge /usr/local/bin/gamebridge-core
rm -rf /usr/local/lib/gamebridge
rm -f /etc/systemd/system/gamebridge@.service
rm -f /etc/systemd/system/gamebridge-kernel@.service
rm -f /etc/systemd/system/gamebridge-forward.service
rm -f /etc/sysctl.d/99-gamebridge-base.conf
rm -f /etc/sysctl.d/99-gamebridge.conf
systemctl daemon-reload

echo
read -r -p "Delete /etc/gamebridge configs too? [y/N] " ans
if [[ "$ans" =~ ^[Yy]$ ]]; then
  rm -rf /etc/gamebridge
fi
echo "GameBridge removed."

#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-only
set -euo pipefail

RAW_BASE="${GAMEBRIDGE_RAW_BASE:-https://raw.githubusercontent.com/devprogrmer/GameBridge}"
REF="${GAMEBRIDGE_INSTALLER_REF:-main}"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

curl -fsSL --retry 3 --connect-timeout 10 \
  -o "$TMP" \
  "${RAW_BASE}/${REF}/install-panel.sh"

GAMEBRIDGE_INSTALL_MODE=agent bash "$TMP"
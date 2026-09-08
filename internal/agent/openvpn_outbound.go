// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const managedOpenVPNOutboundDir = "/etc/gamebridge/openvpn-outbounds"

type OpenVPNOutboundSpec struct {
	Name         string `json:"name"`
	Interface    string `json:"interface"`
	RoutingTable int    `json:"routing_table"`
	Mark         int    `json:"mark"`
	Profile      string `json:"profile"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

type openVPNOutboundSyncRequest struct {
	Outbounds []OpenVPNOutboundSpec `json:"outbounds"`
}

type openVPNOutboundStatus struct {
	Name         string `json:"name"`
	Interface    string `json:"interface"`
	RoutingTable int    `json:"routing_table"`
	Mark         int    `json:"mark"`
	Running      bool   `json:"running"`
}

func safeOpenVPNName(name string) (string, error) {
	name = strings.TrimSpace(name)
	base := filepath.Base(name)
	if base != name || base == "." || base == ".." || !safeName.MatchString(base) {
		return "", errors.New("invalid openvpn outbound name")
	}
	return base, nil
}

func safeOpenVPNInterface(iface string) (string, error) {
	iface = strings.TrimSpace(iface)
	base := filepath.Base(iface)
	if base != iface || base == "." || base == ".." || !safeName.MatchString(base) || len(base) > 15 {
		return "", errors.New("invalid openvpn interface")
	}
	return base, nil
}

func normalizeOpenVPNOutboundSpec(x OpenVPNOutboundSpec) (OpenVPNOutboundSpec, error) {
	var err error
	x.Name, err = safeOpenVPNName(x.Name)
	if err != nil {
		return x, err
	}
	x.Interface, err = safeOpenVPNInterface(x.Interface)
	if err != nil {
		return x, err
	}
	x.Profile = strings.TrimSpace(x.Profile)
	x.Username = strings.TrimSpace(x.Username)

	if x.Profile == "" {
		return x, errors.New("openvpn profile is required")
	}
	if strings.ContainsRune(x.Profile, '\x00') {
		return x, errors.New("openvpn profile contains a NUL byte")
	}
	if x.RoutingTable < 1000 || x.RoutingTable > 65000 {
		return x, errors.New("openvpn routing table must be between 1000 and 65000")
	}
	if x.Mark < 1000 || x.Mark > 65000 {
		return x, errors.New("openvpn mark must be between 1000 and 65000")
	}
	if strings.ContainsAny(x.Username, "\r\n") || strings.ContainsAny(x.Password, "\r\n") {
		return x, errors.New("openvpn username and password must be single-line values")
	}
	if (x.Username == "") != (x.Password == "") {
		return x, errors.New("openvpn username and password must be provided together")
	}
	return x, nil
}

func openVPNConfigPath(name string) (string, error) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedOpenVPNOutboundDir, safe+".ovpn"), nil
}

func openVPNAuthPath(name string) (string, error) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedOpenVPNOutboundDir, safe+".auth"), nil
}

func openVPNMarkerPath(name string) (string, error) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedOpenVPNOutboundDir, safe+".json"), nil
}

func openVPNRouteScriptPath(name string) (string, error) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedOpenVPNOutboundDir, safe+"-route.sh"), nil
}

func openVPNUnitName(name string) (string, error) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return "", err
	}
	return "gamebridge-openvpn-" + safe + ".service", nil
}

func openVPNUnitPath(name string) (string, error) {
	unit, err := openVPNUnitName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join("/etc/systemd/system", unit), nil
}

func sanitizeOpenVPNProfile(profile, iface, authPath string, hasAuth bool) (string, error) {
	safeIface, err := safeOpenVPNInterface(iface)
	if err != nil {
		return "", err
	}

	blocked := map[string]bool{
		"script-security":            true,
		"up":                         true,
		"down":                       true,
		"route-up":                   true,
		"route-pre-down":             true,
		"ipchange":                   true,
		"tls-verify":                 true,
		"auth-user-pass-verify":      true,
		"plugin":                     true,
		"management":                 true,
		"management-client":          true,
		"management-client-auth":     true,
		"management-external-key":    true,
		"management-query-passwords": true,
		"client-connect":             true,
		"client-disconnect":          true,
		"learn-address":              true,
		"daemon":                     true,
		"cd":                         true,
		"chroot":                     true,
		"log":                        true,
		"log-append":                 true,
		"status":                     true,
		"writepid":                   true,
		"tmp-dir":                    true,
		"askpass":                    true,
	}
	blockedRoutes := map[string]bool{
		"route":            true,
		"route-ipv6":       true,
		"redirect-gateway": true,
		"redirect-private": true,
	}
	externalFiles := map[string]bool{
		"ca":           true,
		"capath":       true,
		"cert":         true,
		"key":          true,
		"pkcs12":       true,
		"crl-verify":   true,
		"secret":       true,
		"tls-auth":     true,
		"tls-crypt":    true,
		"tls-crypt-v2": true,
	}
	inlineBlocks := map[string]bool{
		"ca":           true,
		"cert":         true,
		"key":          true,
		"pkcs12":       true,
		"crl-verify":   true,
		"secret":       true,
		"tls-auth":     true,
		"tls-crypt":    true,
		"tls-crypt-v2": true,
	}

	var out []string
	currentInline := ""
	sawAuthDirective := false

	lines := strings.Split(strings.ReplaceAll(profile, "\r\n", "\n"), "\n")
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		lower := strings.ToLower(trimmed)

		if currentInline != "" {
			out = append(out, raw)
			if lower == "</"+currentInline+">" {
				currentInline = ""
			}
			continue
		}

		if strings.HasPrefix(lower, "<") && strings.HasSuffix(lower, ">") && !strings.HasPrefix(lower, "</") {
			tag := strings.TrimSuffix(strings.TrimPrefix(lower, "<"), ">")
			if inlineBlocks[tag] {
				currentInline = tag
				out = append(out, raw)
				continue
			}
			if tag == "connection" {
				return "", errors.New("openvpn <connection> blocks are not supported in managed profiles")
			}
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			out = append(out, raw)
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(strings.TrimLeft(fields[0], "-"))

		if blocked[key] {
			return "", fmt.Errorf("openvpn profile directive %q is not allowed", key)
		}
		if blockedRoutes[key] {
			return "", fmt.Errorf("openvpn profile directive %q would modify host routing", key)
		}
		if externalFiles[key] {
			return "", fmt.Errorf("openvpn profile directive %q must use an inline block", key)
		}

		switch key {
		case "dev":
			if len(fields) > 1 && strings.HasPrefix(strings.ToLower(fields[1]), "tap") {
				return "", errors.New("openvpn TAP profiles are not supported")
			}
			continue
		case "dev-type":
			if len(fields) > 1 && strings.EqualFold(fields[1], "tap") {
				return "", errors.New("openvpn TAP profiles are not supported")
			}
			continue
		case "auth-user-pass":
			sawAuthDirective = true
			continue
		case "route-nopull", "persist-key", "persist-tun", "auth-nocache", "pull-filter":
			continue
		}
		out = append(out, raw)
	}

	if currentInline != "" {
		return "", fmt.Errorf("openvpn inline block <%s> is not closed", currentInline)
	}
	if sawAuthDirective && !hasAuth {
		return "", errors.New("openvpn profile requires username/password authentication")
	}

	out = append(out,
		"",
		"dev "+safeIface,
		"dev-type tun",
		"route-nopull",
		`pull-filter ignore "redirect-gateway"`,
		`pull-filter ignore "redirect-private"`,
		"persist-key",
		"persist-tun",
		"auth-nocache",
	)
	if hasAuth {
		out = append(out, "auth-user-pass "+authPath)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n", nil
}

func renderOpenVPNRouteScript(iface string, table, mark int) (string, error) {
	safeIface, err := safeOpenVPNInterface(iface)
	if err != nil {
		return "", err
	}
	if table < 1000 || table > 65000 || mark < 1000 || mark > 65000 {
		return "", errors.New("invalid openvpn routing values")
	}
	return fmt.Sprintf(`#!/bin/sh
set -u
IFACE=%s
TABLE=%d
MARK=%d

cleanup() {
    ip rule del fwmark "$MARK" table "$TABLE" 2>/dev/null || true
    ip route flush table "$TABLE" 2>/dev/null || true
    ip -6 rule del fwmark "$MARK" table "$TABLE" 2>/dev/null || true
    ip -6 route flush table "$TABLE" 2>/dev/null || true
}

if [ "${1:-up}" = "down" ]; then
    cleanup
    exit 0
fi

i=0
while [ "$i" -lt 120 ]; do
    if ip link show dev "$IFACE" >/dev/null 2>&1; then
        break
    fi
    i=$((i + 1))
    sleep 0.25
done

if ! ip link show dev "$IFACE" >/dev/null 2>&1; then
    exit 1
fi

cleanup
ip route replace default dev "$IFACE" table "$TABLE"
ip rule add fwmark "$MARK" table "$TABLE"
ip -6 route replace default dev "$IFACE" table "$TABLE" 2>/dev/null || true
ip -6 rule add fwmark "$MARK" table "$TABLE" 2>/dev/null || true
`, safeIface, table, mark), nil
}

func renderOpenVPNUnit(binary, configPath, routeScript string) string {
	return fmt.Sprintf(`[Unit]
Description=GameBridge managed OpenVPN outbound
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s
ExecStartPost=%s up
ExecStopPost=%s down
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict

[Install]
WantedBy=multi-user.target
`, binary, configPath, routeScript, routeScript)
}

func openVPNInterfaceReady(iface string) bool {
	safe, err := safeOpenVPNInterface(iface)
	if err != nil {
		return false
	}
	_, err = net.InterfaceByName(safe)
	return err == nil
}

func applyManagedOpenVPNOutbound(x OpenVPNOutboundSpec) error {
	var err error
	x, err = normalizeOpenVPNOutboundSpec(x)
	if err != nil {
		return err
	}

	binary, err := exec.LookPath("openvpn")
	if err != nil {
		return errors.New("openvpn binary not found on node")
	}

	configPath, err := openVPNConfigPath(x.Name)
	if err != nil {
		return err
	}
	authPath, err := openVPNAuthPath(x.Name)
	if err != nil {
		return err
	}
	markerPath, err := openVPNMarkerPath(x.Name)
	if err != nil {
		return err
	}
	routeScriptPath, err := openVPNRouteScriptPath(x.Name)
	if err != nil {
		return err
	}
	unitName, err := openVPNUnitName(x.Name)
	if err != nil {
		return err
	}
	unitPath, err := openVPNUnitPath(x.Name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(managedOpenVPNOutboundDir, 0700); err != nil {
		return err
	}

	hasAuth := x.Username != "" && x.Password != ""
	config, err := sanitizeOpenVPNProfile(x.Profile, x.Interface, authPath, hasAuth)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		return err
	}

	if hasAuth {
		auth := x.Username + "\n" + x.Password + "\n"
		if err := os.WriteFile(authPath, []byte(auth), 0600); err != nil {
			return err
		}
	} else {
		_ = os.Remove(authPath)
	}

	routeScript, err := renderOpenVPNRouteScript(x.Interface, x.RoutingTable, x.Mark)
	if err != nil {
		return err
	}
	if err := os.WriteFile(routeScriptPath, []byte(routeScript), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(renderOpenVPNUnit(binary, configPath, routeScriptPath)), 0644); err != nil {
		return err
	}

	meta, _ := json.Marshal(OpenVPNOutboundSpec{
		Name:         x.Name,
		Interface:    x.Interface,
		RoutingTable: x.RoutingTable,
		Mark:         x.Mark,
	})
	if err := os.WriteFile(markerPath, meta, 0600); err != nil {
		return err
	}

	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", unitName); err != nil {
		return err
	}
	if err := run("systemctl", "restart", unitName); err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		if openVPNInterfaceReady(x.Interface) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("openvpn outbound %s did not create interface %s", x.Name, x.Interface)
}

func removeManagedOpenVPNOutbound(name string) {
	safe, err := safeOpenVPNName(name)
	if err != nil {
		return
	}

	markerPath, err := openVPNMarkerPath(safe)
	if err != nil {
		return
	}
	var meta OpenVPNOutboundSpec
	if b, readErr := os.ReadFile(markerPath); readErr == nil {
		_ = json.Unmarshal(b, &meta)
	}

	unitName, err := openVPNUnitName(safe)
	if err != nil {
		return
	}
	unitPath, err := openVPNUnitPath(safe)
	if err != nil {
		return
	}
	configPath, err := openVPNConfigPath(safe)
	if err != nil {
		return
	}
	authPath, err := openVPNAuthPath(safe)
	if err != nil {
		return
	}
	routeScriptPath, err := openVPNRouteScriptPath(safe)
	if err != nil {
		return
	}

	_ = run("systemctl", "disable", "--now", unitName)
	if meta.Mark >= 1000 && meta.RoutingTable >= 1000 {
		_ = run("ip", "rule", "del", "fwmark", strconv.Itoa(meta.Mark), "table", strconv.Itoa(meta.RoutingTable))
		_ = run("ip", "route", "flush", "table", strconv.Itoa(meta.RoutingTable))
	}
	_ = os.Remove(unitPath)
	_ = os.Remove(configPath)
	_ = os.Remove(authPath)
	_ = os.Remove(markerPath)
	_ = os.Remove(routeScriptPath)
	_ = run("systemctl", "daemon-reload")
}

func syncManagedOpenVPNOutbounds(specs []OpenVPNOutboundSpec) error {
	normalized := make([]OpenVPNOutboundSpec, 0, len(specs))
	desired := map[string]bool{}
	ifaces := map[string]bool{}
	tables := map[int]bool{}
	marks := map[int]bool{}

	for _, raw := range specs {
		x, err := normalizeOpenVPNOutboundSpec(raw)
		if err != nil {
			return err
		}
		if desired[x.Name] {
			return fmt.Errorf("duplicate openvpn outbound name %s", x.Name)
		}
		if ifaces[x.Interface] {
			return fmt.Errorf("duplicate openvpn interface %s", x.Interface)
		}
		if tables[x.RoutingTable] {
			return fmt.Errorf("duplicate openvpn routing table %d", x.RoutingTable)
		}
		if marks[x.Mark] {
			return fmt.Errorf("duplicate openvpn mark %d", x.Mark)
		}
		desired[x.Name] = true
		ifaces[x.Interface] = true
		tables[x.RoutingTable] = true
		marks[x.Mark] = true
		normalized = append(normalized, x)
	}

	for _, x := range normalized {
		if err := applyManagedOpenVPNOutbound(x); err != nil {
			return fmt.Errorf("apply openvpn outbound %s: %w", x.Name, err)
		}
	}

	markers, _ := filepath.Glob(filepath.Join(managedOpenVPNOutboundDir, "*.json"))
	for _, marker := range markers {
		name := strings.TrimSuffix(filepath.Base(marker), ".json")
		if !desired[name] {
			removeManagedOpenVPNOutbound(name)
		}
	}
	return nil
}

func listManagedOpenVPNOutbounds() []openVPNOutboundStatus {
	markers, _ := filepath.Glob(filepath.Join(managedOpenVPNOutboundDir, "*.json"))
	out := make([]openVPNOutboundStatus, 0, len(markers))
	for _, marker := range markers {
		b, err := os.ReadFile(marker)
		if err != nil {
			continue
		}
		var spec OpenVPNOutboundSpec
		if json.Unmarshal(b, &spec) != nil {
			continue
		}
		if _, err := safeOpenVPNName(spec.Name); err != nil {
			continue
		}
		if _, err := safeOpenVPNInterface(spec.Interface); err != nil {
			continue
		}
		out = append(out, openVPNOutboundStatus{
			Name:         spec.Name,
			Interface:    spec.Interface,
			RoutingTable: spec.RoutingTable,
			Mark:         spec.Mark,
			Running:      openVPNInterfaceReady(spec.Interface),
		})
	}
	return out
}

func (s *Server) handleOpenVPNOutboundSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req openVPNOutboundSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := syncManagedOpenVPNOutbounds(req.Outbounds); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "count": len(req.Outbounds)})
}

func (s *Server) handleOpenVPNStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, err := exec.LookPath("openvpn")
	jsonWrite(w, http.StatusOK, map[string]any{
		"installed": err == nil,
		"outbounds": listManagedOpenVPNOutbounds(),
	})
}

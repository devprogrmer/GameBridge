// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const managedWGOutboundDir = "/etc/gamebridge/wireguard-outbounds"

type WGOutboundSpec struct {
	Interface           string   `json:"interface"`
	Address             string   `json:"address"`
	PrivateKey          string   `json:"private_key"`
	PeerPublicKey       string   `json:"peer_public_key"`
	Endpoint            string   `json:"endpoint"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
	MTU                 int      `json:"mtu"`
}

type wgOutboundSyncRequest struct {
	Outbounds []WGOutboundSpec `json:"outbounds"`
}

func normalizeWGOutboundSpec(x WGOutboundSpec) (WGOutboundSpec, error) {
	x.Interface = strings.TrimSpace(x.Interface)
	x.Address = strings.TrimSpace(x.Address)
	x.PrivateKey = strings.TrimSpace(x.PrivateKey)
	x.PeerPublicKey = strings.TrimSpace(x.PeerPublicKey)
	x.Endpoint = strings.TrimSpace(x.Endpoint)
	for i := range x.AllowedIPs {
		x.AllowedIPs[i] = strings.TrimSpace(x.AllowedIPs[i])
	}

	if !safeName.MatchString(x.Interface) || len(x.Interface) > 15 {
		return x, errors.New("invalid WireGuard outbound interface")
	}
	if _, _, err := net.ParseCIDR(x.Address); err != nil {
		return x, errors.New("invalid WireGuard outbound address")
	}
	if x.PrivateKey == "" || x.PeerPublicKey == "" {
		return x, errors.New("WireGuard private_key and peer_public_key are required")
	}
	host, port, err := net.SplitHostPort(x.Endpoint)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return x, errors.New("invalid WireGuard endpoint")
	}
	if len(x.AllowedIPs) == 0 {
		x.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	for _, cidr := range x.AllowedIPs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return x, fmt.Errorf("invalid WireGuard allowed IP %q", cidr)
		}
	}
	if x.PersistentKeepalive == 0 {
		x.PersistentKeepalive = 25
	}
	if x.PersistentKeepalive < 0 || x.PersistentKeepalive > 65535 {
		return x, errors.New("invalid WireGuard keepalive")
	}
	if x.MTU == 0 {
		x.MTU = 1420
	}
	if x.MTU < 576 || x.MTU > 9000 {
		return x, errors.New("invalid WireGuard MTU")
	}
	return x, nil
}

func renderWGOutboundConfig(x WGOutboundSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\nAddress = %s\nPrivateKey = %s\nMTU = %d\nTable = off\n\n", x.Address, x.PrivateKey, x.MTU)
	fmt.Fprintf(&b, "[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = %d\n",
		x.PeerPublicKey, x.Endpoint, strings.Join(x.AllowedIPs, ", "), x.PersistentKeepalive)
	return b.String()
}

func safeWGPathComponent(iface string) (string, error) {
	iface = strings.TrimSpace(iface)
	base := filepath.Base(iface)
	if base != iface || base == "." || base == ".." || !safeName.MatchString(base) || len(base) > 15 {
		return "", errors.New("invalid WireGuard outbound interface")
	}
	return base, nil
}

func managedWGMarker(iface string) (string, error) {
	safeIface, err := safeWGPathComponent(iface)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedWGOutboundDir, safeIface+".json"), nil
}

func wgConfigPath(iface string) (string, error) {
	safeIface, err := safeWGPathComponent(iface)
	if err != nil {
		return "", err
	}
	return filepath.Join("/etc/wireguard", safeIface+".conf"), nil
}

func applyManagedWGOutbound(x WGOutboundSpec) error {
	var err error
	x, err = normalizeWGOutboundSpec(x)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(managedWGOutboundDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll("/etc/wireguard", 0700); err != nil {
		return err
	}

	cfgPath, err := wgConfigPath(x.Interface)
	if err != nil {
		return err
	}
	marker, err := managedWGMarker(x.Interface)
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfgPath); err == nil {
		if _, markerErr := os.Stat(marker); markerErr != nil {
			return fmt.Errorf("wireguard interface %s already has an unmanaged config", x.Interface)
		}
	}

	tmp := cfgPath + ".gamebridge.tmp"
	if err := os.WriteFile(tmp, []byte(renderWGOutboundConfig(x)), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	meta, _ := json.Marshal(map[string]string{"interface": x.Interface})
	if err := os.WriteFile(marker, meta, 0600); err != nil {
		return err
	}
	unit := "wg-quick@" + x.Interface + ".service"
	if err := run("systemctl", "enable", unit); err != nil {
		return err
	}
	if err := run("systemctl", "restart", unit); err != nil {
		return err
	}
	return nil
}

func removeManagedWGOutbound(iface string) {
	safeIface, err := safeWGPathComponent(iface)
	if err != nil {
		return
	}
	cfgPath, err := wgConfigPath(safeIface)
	if err != nil {
		return
	}
	marker, err := managedWGMarker(safeIface)
	if err != nil {
		return
	}
	unit := "wg-quick@" + safeIface + ".service"
	_ = run("systemctl", "disable", "--now", unit)
	_ = os.Remove(cfgPath)
	_ = os.Remove(marker)
}

func syncManagedWGOutbounds(specs []WGOutboundSpec) error {
	normalized := make([]WGOutboundSpec, 0, len(specs))
	desired := map[string]bool{}
	for _, raw := range specs {
		x, err := normalizeWGOutboundSpec(raw)
		if err != nil {
			return err
		}
		if desired[x.Interface] {
			return fmt.Errorf("duplicate WireGuard interface %s", x.Interface)
		}
		desired[x.Interface] = true
		normalized = append(normalized, x)
	}

	for _, x := range normalized {
		if err := applyManagedWGOutbound(x); err != nil {
			return fmt.Errorf("apply %s: %w", x.Interface, err)
		}
	}

	markers, _ := filepath.Glob(filepath.Join(managedWGOutboundDir, "*.json"))
	for _, marker := range markers {
		iface := strings.TrimSuffix(filepath.Base(marker), ".json")
		if !desired[iface] {
			removeManagedWGOutbound(iface)
		}
	}
	return nil
}

func (s *Server) handleWGOutboundSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req wgOutboundSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := syncManagedWGOutbounds(req.Outbounds); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "count": len(req.Outbounds)})
}

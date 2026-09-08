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

const (
	managedTorOutboundDir = "/etc/gamebridge/tor-outbounds"
	managedTorDataDir     = "/var/lib/gamebridge/tor-outbounds"
)

type TorOutboundSpec struct {
	Name      string `json:"name"`
	SocksPort int    `json:"socks_port"`
}

type torOutboundSyncRequest struct {
	Outbounds []TorOutboundSpec `json:"outbounds"`
}

type torOutboundStatus struct {
	Name      string `json:"name"`
	SocksPort int    `json:"socks_port"`
	Running   bool   `json:"running"`
}

func safeTorName(name string) (string, error) {
	name = strings.TrimSpace(name)
	base := filepath.Base(name)
	if base != name || base == "." || base == ".." || !safeName.MatchString(base) {
		return "", errors.New("invalid Tor outbound name")
	}
	return base, nil
}

func normalizeTorOutboundSpec(x TorOutboundSpec) (TorOutboundSpec, error) {
	var err error
	x.Name, err = safeTorName(x.Name)
	if err != nil {
		return x, err
	}
	if x.SocksPort < 1024 || x.SocksPort > 65535 {
		return x, errors.New("tor SOCKS port must be between 1024 and 65535")
	}
	return x, nil
}

func torConfigPath(name string) (string, error) {
	safe, err := safeTorName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedTorOutboundDir, safe+".torrc"), nil
}

func torMarkerPath(name string) (string, error) {
	safe, err := safeTorName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedTorOutboundDir, safe+".json"), nil
}

func torDataPath(name string) (string, error) {
	safe, err := safeTorName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(managedTorDataDir, safe), nil
}

func torUnitName(name string) (string, error) {
	safe, err := safeTorName(name)
	if err != nil {
		return "", err
	}
	return "gamebridge-tor-" + safe + ".service", nil
}

func torUnitPath(name string) (string, error) {
	unit, err := torUnitName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join("/etc/systemd/system", unit), nil
}

func renderTorConfig(x TorOutboundSpec, dataDir string) string {
	return fmt.Sprintf(
		"ClientOnly 1\nSocksPort 127.0.0.1:%d IsolateSOCKSAuth\nDataDirectory %s\nAvoidDiskWrites 1\n",
		x.SocksPort, dataDir,
	)
}

func renderTorUnit(torBinary, configPath, dataDir string) string {
	return fmt.Sprintf(`[Unit]
Description=GameBridge managed Tor outbound
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s -f %s
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=%s
ReadOnlyPaths=%s

[Install]
WantedBy=multi-user.target
`, torBinary, configPath, dataDir, configPath)
}

func torSOCKSReady(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 1200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func applyManagedTorOutbound(x TorOutboundSpec) error {
	var err error
	x, err = normalizeTorOutboundSpec(x)
	if err != nil {
		return err
	}

	torBinary, err := exec.LookPath("tor")
	if err != nil {
		return errors.New("tor binary not found on node")
	}

	configPath, err := torConfigPath(x.Name)
	if err != nil {
		return err
	}
	markerPath, err := torMarkerPath(x.Name)
	if err != nil {
		return err
	}
	dataDir, err := torDataPath(x.Name)
	if err != nil {
		return err
	}
	unitPath, err := torUnitPath(x.Name)
	if err != nil {
		return err
	}
	unitName, err := torUnitName(x.Name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(managedTorOutboundDir, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}

	if err := os.WriteFile(configPath, []byte(renderTorConfig(x, dataDir)), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(renderTorUnit(torBinary, configPath, dataDir)), 0644); err != nil {
		return err
	}

	meta, _ := json.Marshal(x)
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

	for i := 0; i < 12; i++ {
		if torSOCKSReady(x.SocksPort) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("tor outbound %s did not open SOCKS port %d", x.Name, x.SocksPort)
}

func removeManagedTorOutbound(name string) {
	safe, err := safeTorName(name)
	if err != nil {
		return
	}
	unitName, err := torUnitName(safe)
	if err != nil {
		return
	}
	unitPath, err := torUnitPath(safe)
	if err != nil {
		return
	}
	configPath, err := torConfigPath(safe)
	if err != nil {
		return
	}
	markerPath, err := torMarkerPath(safe)
	if err != nil {
		return
	}
	dataDir, err := torDataPath(safe)
	if err != nil {
		return
	}

	_ = run("systemctl", "disable", "--now", unitName)
	_ = os.Remove(unitPath)
	_ = os.Remove(configPath)
	_ = os.Remove(markerPath)
	_ = os.RemoveAll(dataDir)
	_ = run("systemctl", "daemon-reload")
}

func syncManagedTorOutbounds(specs []TorOutboundSpec) error {
	normalized := make([]TorOutboundSpec, 0, len(specs))
	desired := map[string]bool{}
	ports := map[int]bool{}

	for _, raw := range specs {
		x, err := normalizeTorOutboundSpec(raw)
		if err != nil {
			return err
		}
		if desired[x.Name] {
			return fmt.Errorf("duplicate Tor outbound name %s", x.Name)
		}
		if ports[x.SocksPort] {
			return fmt.Errorf("duplicate Tor SOCKS port %d", x.SocksPort)
		}
		desired[x.Name] = true
		ports[x.SocksPort] = true
		normalized = append(normalized, x)
	}

	for _, x := range normalized {
		if err := applyManagedTorOutbound(x); err != nil {
			return fmt.Errorf("apply Tor outbound %s: %w", x.Name, err)
		}
	}

	markers, _ := filepath.Glob(filepath.Join(managedTorOutboundDir, "*.json"))
	for _, marker := range markers {
		name := strings.TrimSuffix(filepath.Base(marker), ".json")
		if !desired[name] {
			removeManagedTorOutbound(name)
		}
	}
	return nil
}

func listManagedTorOutbounds() []torOutboundStatus {
	markers, _ := filepath.Glob(filepath.Join(managedTorOutboundDir, "*.json"))
	out := make([]torOutboundStatus, 0, len(markers))
	for _, marker := range markers {
		b, err := os.ReadFile(marker)
		if err != nil {
			continue
		}
		var spec TorOutboundSpec
		if json.Unmarshal(b, &spec) != nil {
			continue
		}
		if _, err := normalizeTorOutboundSpec(spec); err != nil {
			continue
		}
		out = append(out, torOutboundStatus{
			Name:      spec.Name,
			SocksPort: spec.SocksPort,
			Running:   torSOCKSReady(spec.SocksPort),
		})
	}
	return out
}

func (s *Server) handleTorOutboundSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req torOutboundSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := syncManagedTorOutbounds(req.Outbounds); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "count": len(req.Outbounds)})
}

func (s *Server) handleTorStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, err := exec.LookPath("tor")
	jsonWrite(w, http.StatusOK, map[string]any{
		"installed": err == nil,
		"outbounds": listManagedTorOutbounds(),
	})
}

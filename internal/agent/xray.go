// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const xrayConfigPath = "/usr/local/etc/xray/config.json"

func (s *Server) registerXrayRoutes() {
	s.mux.HandleFunc("/v1/xray/status", s.auth(s.handleXrayStatus))
	s.mux.HandleFunc("/v1/xray/install", s.auth(s.handleXrayInstall))
	s.mux.HandleFunc("/v1/xray/x25519", s.auth(s.handleXrayX25519))
	s.mux.HandleFunc("/v1/xray/apply", s.auth(s.handleXrayApply))
	s.mux.HandleFunc("/v1/xray/outbound-probe", s.auth(s.handleXrayOutboundProbe))
}

func xrayBinary() string {
	if p, err := exec.LookPath("xray"); err == nil {
		return p
	}
	if _, err := os.Stat("/usr/local/bin/xray"); err == nil {
		return "/usr/local/bin/xray"
	}
	return ""
}

func xrayStatus() map[string]any {
	bin := xrayBinary()
	installed := bin != ""
	version := ""
	if installed {
		lines := strings.Split(strings.TrimSpace(runOutput(bin, "version")), "\n")
		if len(lines) > 0 {
			version = lines[0]
		}
	}
	active := strings.TrimSpace(runOutput("systemctl", "is-active", "xray.service")) == "active"
	_, cfgErr := os.Stat(xrayConfigPath)
	return map[string]any{
		"installed":     installed,
		"binary":        bin,
		"version":       version,
		"active":        active,
		"config_path":   xrayConfigPath,
		"config_exists": cfgErr == nil,
	}
}

func (s *Server) handleXrayStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jsonWrite(w, http.StatusOK, xrayStatus())
}

func (s *Server) handleXrayInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if xrayBinary() == "" {
		cmd := exec.Command("bash", "-c", `set -euo pipefail; curl -L --fail --retry 3 https://github.com/XTLS/Xray-install/raw/main/install-release.sh | bash -s -- install`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("xray install failed: %v: %s", err, strings.TrimSpace(string(out))))
			return
		}
	}
	jsonWrite(w, http.StatusOK, xrayStatus())
}

func parseX25519Output(out string) (string, string) {
	var privateKey, publicKey string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(parts[0]), " ", ""))
		v := strings.TrimSpace(parts[1])
		switch k {
		case "privatekey", "private":
			privateKey = v
		case "publickey", "public", "password":
			publicKey = v
		}
	}
	return privateKey, publicKey
}

func (s *Server) handleXrayX25519(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bin := xrayBinary()
	if bin == "" {
		jsonError(w, http.StatusConflict, "xray is not installed")
		return
	}
	out, err := exec.Command(bin, "x25519").CombinedOutput()
	if err != nil {
		jsonError(w, http.StatusBadGateway, fmt.Sprintf("x25519 failed: %v: %s", err, strings.TrimSpace(string(out))))
		return
	}
	priv, pub := parseX25519Output(string(out))
	if priv == "" || pub == "" {
		jsonError(w, http.StatusBadGateway, "could not parse xray x25519 output")
		return
	}
	jsonWrite(w, http.StatusOK, map[string]string{"private_key": priv, "public_key": pub})
}

func (s *Server) handleXrayApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bin := xrayBinary()
	if bin == "" {
		jsonError(w, http.StatusConflict, "xray is not installed")
		return
	}
	var in struct {
		Config json.RawMessage `json:"config"`
	}
	if err := decodeJSON(r, &in); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(in.Config) == 0 || !json.Valid(in.Config) {
		jsonError(w, http.StatusBadRequest, "invalid xray config")
		return
	}

	if err := os.MkdirAll(filepath.Dir(xrayConfigPath), 0755); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tmp := fmt.Sprintf("%s.gbtmp-%d", xrayConfigPath, time.Now().UnixNano())
	if err := os.WriteFile(tmp, append(in.Config, '\n'), 0600); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmp)

	testOut, err := exec.Command(bin, "run", "-test", "-config", tmp).CombinedOutput()
	if err != nil {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("xray config test failed: %v: %s", err, strings.TrimSpace(string(testOut))))
		return
	}

	backup := xrayConfigPath + ".gamebridge.bak"
	hadOld := false
	if _, err := os.Stat(xrayConfigPath); err == nil {
		hadOld = true
		old, err := os.ReadFile(xrayConfigPath)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.WriteFile(backup, old, 0600); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	raw, err := os.ReadFile(tmp)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.WriteFile(xrayConfigPath, raw, 0600); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_ = exec.Command("systemctl", "enable", "xray.service").Run()
	restartOut, restartErr := exec.Command("systemctl", "restart", "xray.service").CombinedOutput()
	if restartErr != nil {
		if hadOld {
			if old, err := os.ReadFile(backup); err == nil {
				_ = os.WriteFile(xrayConfigPath, old, 0600)
				_ = exec.Command("systemctl", "restart", "xray.service").Run()
			}
		}
		jsonError(w, http.StatusBadGateway, fmt.Sprintf("xray restart failed: %v: %s", restartErr, strings.TrimSpace(string(restartOut))))
		return
	}

	jsonWrite(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": xrayStatus(),
	})
}

var _ = errors.New

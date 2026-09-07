// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

func (s *Server) registerPhase3Routes() {
	s.mux.HandleFunc("/v1/xray/stats", s.auth(s.handleXrayStats))
	s.mux.HandleFunc("/v1/wireguard/peer-state", s.auth(s.handleWGPeerState))
	s.mux.HandleFunc("/v1/wireguard/runtime", s.auth(s.handleWGRuntime))
}

type rawXrayStatResponse struct {
	Stat []struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	} `json:"stat"`
}

type normalizedXrayStat struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
}

func parseXrayStatValue(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		n, _ = strconv.ParseInt(s, 10, 64)
		return n
	}
	return 0
}

func normalizeXrayUserStats(raw []byte) ([]normalizedXrayStat, error) {
	var resp rawXrayStatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	byEmail := map[string]*normalizedXrayStat{}
	order := []string{}
	for _, st := range resp.Stat {
		parts := strings.Split(st.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		email := parts[1]
		if _, ok := byEmail[email]; !ok {
			byEmail[email] = &normalizedXrayStat{Email: email}
			order = append(order, email)
		}
		v := parseXrayStatValue(st.Value)
		switch parts[3] {
		case "uplink":
			byEmail[email].Uplink += v
		case "downlink":
			byEmail[email].Downlink += v
		}
	}
	out := make([]normalizedXrayStat, 0, len(order))
	for _, email := range order {
		out = append(out, *byEmail[email])
	}
	return out, nil
}

func (s *Server) handleXrayStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bin := xrayBinary()
	if bin == "" {
		jsonError(w, http.StatusConflict, "xray is not installed")
		return
	}
	args := []string{"api", "statsquery", "--server=127.0.0.1:10085", "-pattern", "user>>>"}
	if r.URL.Query().Get("reset") == "1" {
		args = append(args, "-reset=true")
	}
	raw, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		jsonError(w, http.StatusBadGateway, fmt.Sprintf("xray stats query failed: %v: %s", err, strings.TrimSpace(string(raw))))
		return
	}
	users, err := normalizeXrayUserStats(raw)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "invalid xray stats output: "+err.Error())
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{"users": users})
}

type wgRuntimePeer struct {
	Interface       string `json:"interface"`
	PublicKey       string `json:"public_key"`
	LatestHandshake int64  `json:"latest_handshake"`
	RXBytes         int64  `json:"rx_bytes"`
	TXBytes         int64  `json:"tx_bytes"`
}

func parseWGRuntimeDump(out string) []wgRuntimePeer {
	var peers []wgRuntimePeer
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		handshake, _ := strconv.ParseInt(fields[5], 10, 64)
		rx, _ := strconv.ParseInt(fields[6], 10, 64)
		tx, _ := strconv.ParseInt(fields[7], 10, 64)
		peers = append(peers, wgRuntimePeer{
			Interface:       fields[0],
			PublicKey:       fields[1],
			LatestHandshake: handshake,
			RXBytes:         rx,
			TXBytes:         tx,
		})
	}
	return peers
}

func (s *Server) handleWGRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	jsonWrite(w, http.StatusOK, map[string]any{
		"peers": parseWGRuntimeDump(runOutput("wg", "show", "all", "dump")),
	})
}

type wgPeerStateRequest struct {
	Interface     string `json:"interface"`
	PublicKey     string `json:"public_key"`
	ClientAddress string `json:"client_address"`
	Enabled       bool   `json:"enabled"`
}

func (s *Server) handleWGPeerState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var in wgPeerStateRequest
	if err := decodeJSON(r, &in); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !safeName.MatchString(in.Interface) || strings.TrimSpace(in.PublicKey) == "" {
		jsonError(w, http.StatusBadRequest, "invalid WireGuard peer state request")
		return
	}

	if in.Enabled {
		if strings.TrimSpace(in.ClientAddress) == "" {
			jsonError(w, http.StatusBadRequest, "client_address is required when enabling a peer")
			return
		}
		if err := run("wg", "set", in.Interface, "peer", in.PublicKey, "allowed-ips", in.ClientAddress); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
	} else {
		if err := run("wg", "set", in.Interface, "peer", in.PublicKey, "remove"); err != nil {
			// Removing an already absent peer is effectively the desired state.
			if !strings.Contains(strings.ToLower(err.Error()), "not found") {
				jsonError(w, http.StatusBadGateway, err.Error())
				return
			}
		}
	}
	_ = exec.Command("wg-quick", "save", in.Interface).Run()
	jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
}

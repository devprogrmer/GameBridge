// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

type normalizedXrayUserStat struct {
	Email    string `json:"email"`
	Uplink   int64  `json:"uplink"`
	Downlink int64  `json:"downlink"`
}

type normalizedXrayStatsResponse struct {
	Users []normalizedXrayUserStat `json:"users"`
}

type wgRuntimeStat struct {
	Interface       string `json:"interface"`
	PublicKey       string `json:"public_key"`
	LatestHandshake int64  `json:"latest_handshake"`
	RXBytes         int64  `json:"rx_bytes"`
	TXBytes         int64  `json:"tx_bytes"`
}

type peerStateTask struct {
	Peer    VPNPeer
	Enabled bool
}

func nonNegative(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func (s *Server) syncWireGuard() {
	var nodes []Node
	_ = s.store.Read(func(st State) error {
		nodes = st.Nodes
		return nil
	})

	seen := map[string]wgRuntimeStat{}
	for _, n := range nodes {
		if !n.Enabled {
			continue
		}
		var resp struct {
			Peers []wgRuntimeStat `json:"peers"`
		}
		if s.agentJSON(n, http.MethodGet, "/v1/wireguard/runtime", nil, &resp) == nil {
			for _, p := range resp.Peers {
				seen[n.ID+"|"+p.PublicKey] = p
			}
		}
	}

	now := time.Now().UTC()
	_ = s.store.Update(func(st *State) error {
		wgActive := map[string]bool{}
		for i := range st.VPNPeers {
			p := &st.VPNPeers[i]
			if x, ok := seen[p.NodeID+"|"+p.PublicKey]; ok {
				counterMoved := x.RXBytes > p.RXBytes || x.TXBytes > p.TXBytes
				recentHandshake := x.LatestHandshake > 0 && now.Unix()-x.LatestHandshake <= 180
				p.RXBytes = x.RXBytes
				p.TXBytes = x.TXBytes
				if p.Enabled && (counterMoved || recentHandshake) {
					wgActive[p.UserID] = true
				}
			}
		}
		for i := range st.Users {
			u := &st.Users[i]
			var total int64
			for _, p := range st.VPNPeers {
				if p.UserID == u.ID {
					total += nonNegative(p.RXBytes-p.RXBaseBytes) + nonNegative(p.TXBytes-p.TXBaseBytes)
				}
			}
			u.WireGuardTrafficBytes = total
			u.TrafficUsedBytes = u.WireGuardTrafficBytes + u.XrayTrafficBytes
			if wgActive[u.ID] {
				u.LastOnlineAt = now
				u.Online = true
			}
		}
		return nil
	})

	s.syncXrayTraffic()
	s.processTrafficResets()
	s.enforceUserPolicies()
}

func (s *Server) syncXrayTraffic() {
	var nodes []Node
	emailToUser := map[string]string{}
	_ = s.store.Read(func(st State) error {
		nodes = st.Nodes
		for _, c := range st.InboundClients {
			if c.Email != "" {
				emailToUser[c.Email] = c.UserID
			}
		}
		return nil
	})

	deltas := map[string]int64{}
	for _, n := range nodes {
		if !n.Enabled {
			continue
		}
		var resp normalizedXrayStatsResponse
		if err := s.agentJSON(n, http.MethodGet, "/v1/xray/stats?reset=1", nil, &resp); err != nil {
			continue
		}
		for _, x := range resp.Users {
			userID := emailToUser[x.Email]
			if userID == "" {
				continue
			}
			deltas[userID] += nonNegative(x.Uplink) + nonNegative(x.Downlink)
		}
	}

	now := time.Now().UTC()
	_ = s.store.Update(func(st *State) error {
		for i := range st.Users {
			u := &st.Users[i]
			if delta := deltas[u.ID]; delta > 0 {
				u.XrayTrafficBytes += delta
				u.LastOnlineAt = now
				u.Online = true
			} else {
				u.Online = !u.LastOnlineAt.IsZero() && now.Sub(u.LastOnlineAt) <= 90*time.Second
				if !u.Online {
					u.OnlineIPs = 0
				}
			}
			u.TrafficUsedBytes = u.WireGuardTrafficBytes + u.XrayTrafficBytes
		}
		return nil
	})
}

func (s *Server) processTrafficResets() {
	now := time.Now().UTC()
	_ = s.store.Update(func(st *State) error {
		for i := range st.Users {
			u := &st.Users[i]
			days := effectiveResetIntervalDays(st, *u)
			if days <= 0 {
				u.NextTrafficResetAt = time.Time{}
				continue
			}
			if u.NextTrafficResetAt.IsZero() {
				u.NextTrafficResetAt = now.Add(time.Duration(days) * 24 * time.Hour)
				continue
			}
			if now.Before(u.NextTrafficResetAt) {
				continue
			}

			u.XrayTrafficBytes = 0
			u.WireGuardTrafficBytes = 0
			u.TrafficUsedBytes = 0
			if u.Status == "quota-exceeded" {
				u.Status = "active"
			}
			for j := range st.VPNPeers {
				p := &st.VPNPeers[j]
				if p.UserID == u.ID {
					p.RXBaseBytes = p.RXBytes
					p.TXBaseBytes = p.TXBytes
				}
			}
			next := u.NextTrafficResetAt
			step := time.Duration(days) * 24 * time.Hour
			for !next.After(now) {
				next = next.Add(step)
			}
			u.NextTrafficResetAt = next
			u.UpdatedAt = now
		}
		return nil
	})
}

func (s *Server) enforceUserPolicies() {
	now := time.Now().UTC()
	nodeRedeploy := map[string]bool{}
	var peerTasks []peerStateTask

	_ = s.store.Update(func(st *State) error {
		statusChanged := map[string]bool{}
		for i := range st.Users {
			u := &st.Users[i]
			u.TrafficUsedBytes = u.WireGuardTrafficBytes + u.XrayTrafficBytes
			limit := effectiveUserLimit(st, *u)
			expired := !u.ExpiresAt.IsZero() && now.After(u.ExpiresAt)
			old := u.Status

			switch {
			case expired:
				u.Status = "expired"
			case limit > 0 && u.TrafficUsedBytes >= limit:
				u.Status = "quota-exceeded"
			case u.Status == "quota-exceeded":
				u.Status = "active"
			}
			if old != u.Status {
				statusChanged[u.ID] = true
				u.UpdatedAt = now
			}
		}

		for i := range st.InboundClients {
			c := &st.InboundClients[i]
			u := findUser(st, c.UserID)
			if u == nil {
				continue
			}
			desired := u.Status == "active"
			if c.Enabled != desired {
				c.Enabled = desired
				if in := findInbound(st, c.InboundID); in != nil {
					nodeRedeploy[in.NodeID] = true
				}
			} else if statusChanged[u.ID] {
				if in := findInbound(st, c.InboundID); in != nil {
					nodeRedeploy[in.NodeID] = true
				}
			}
		}

		for i := range st.VPNPeers {
			p := &st.VPNPeers[i]
			u := findUser(st, p.UserID)
			if u == nil {
				continue
			}
			desired := u.Status == "active"
			if p.Enabled != desired {
				p.Enabled = desired
				peerTasks = append(peerTasks, peerStateTask{Peer: *p, Enabled: desired})
			}
		}
		return nil
	})

	for nodeID := range nodeRedeploy {
		_ = s.deployXrayNode(nodeID)
	}
	for _, task := range peerTasks {
		if err := s.setRemotePeerEnabled(task.Peer, task.Enabled); err != nil {
			_ = s.store.Update(func(st *State) error {
				if p := findPeer(st, task.Peer.ID); p != nil {
					p.Enabled = !task.Enabled
				}
				return nil
			})
			continue
		}
		if task.Enabled {
			_ = s.store.Update(func(st *State) error {
				if p := findPeer(st, task.Peer.ID); p != nil {
					// Re-adding a WireGuard peer starts its kernel counters from zero.
					p.RXBytes = 0
					p.TXBytes = 0
					p.RXBaseBytes = 0
					p.TXBaseBytes = 0
				}
				return nil
			})
		}
	}
}

func (s *Server) setRemotePeerEnabled(p VPNPeer, enabled bool) error {
	var n Node
	if err := s.store.Read(func(st State) error {
		x := findNode(&st, p.NodeID)
		if x == nil {
			return errors.New("node missing")
		}
		n = *x
		return nil
	}); err != nil {
		return err
	}
	return s.agentJSON(n, http.MethodPost, "/v1/wireguard/peer-state", map[string]any{
		"interface":      p.Interface,
		"public_key":     p.PublicKey,
		"client_address": p.Address,
		"enabled":        enabled,
	}, nil)
}

func isIPv4Broadcast(ip net.IP, network *net.IPNet) bool {
	v := ip.To4()
	base := network.IP.To4()
	if v == nil || base == nil || len(network.Mask) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if v[i] != (base[i] | ^network.Mask[i]) {
			return false
		}
	}
	return true
}

func nextWGAddress(serverAddress string, used map[string]bool) (string, error) {
	serverIP, network, err := net.ParseCIDR(serverAddress)
	if err != nil {
		return "", err
	}
	candidate := append(net.IP(nil), serverIP...)
	bits := 128
	if serverIP.To4() != nil {
		candidate = append(net.IP(nil), serverIP.To4()...)
		bits = 32
	}
	for attempts := 0; attempts < 65536; attempts++ {
		incIP(candidate)
		if !network.Contains(candidate) {
			break
		}
		if candidate.Equal(network.IP) || isIPv4Broadcast(candidate, network) {
			continue
		}
		if !used[candidate.String()] {
			return candidate.String() + "/" + strconv.Itoa(bits), nil
		}
	}
	return "", errors.New("WireGuard subnet exhausted")
}

func validateWGClientAddress(serverAddress, clientAddress string, used map[string]bool) (string, error) {
	serverIP, network, err := net.ParseCIDR(serverAddress)
	if err != nil {
		return "", err
	}
	clientIP, _, err := net.ParseCIDR(clientAddress)
	if err != nil {
		return "", errors.New("invalid client_address")
	}
	if serverIP.To4() != nil {
		clientIP = clientIP.To4()
	}
	if clientIP == nil || !network.Contains(clientIP) {
		return "", errors.New("client_address is outside WireGuard subnet")
	}
	if clientIP.Equal(serverIP) || clientIP.Equal(network.IP) || isIPv4Broadcast(clientIP, network) {
		return "", errors.New("client_address is reserved")
	}
	if used[clientIP.String()] {
		return "", errors.New("client_address is already in use")
	}
	bits := 128
	if clientIP.To4() != nil {
		bits = 32
	}
	return clientIP.String() + "/" + strconv.Itoa(bits), nil
}

func (s *Server) createWGPeer(userID string, in wgCreate) (VPNPeer, string, error) {
	var u User
	var n Node
	deviceCount := 0
	used := map[string]bool{}

	if err := s.store.Read(func(st State) error {
		x := findUser(&st, userID)
		if x == nil {
			return errors.New("user not found")
		}
		u = *x
		y := findNode(&st, in.NodeID)
		if y == nil {
			return errors.New("node not found")
		}
		n = *y

		for _, p := range st.VPNPeers {
			if p.UserID == userID {
				deviceCount++
			}
			if p.NodeID == in.NodeID && p.Interface == in.Interface {
				if ip, _, err := net.ParseCIDR(p.Address); err == nil {
					used[ip.String()] = true
				}
			}
		}
		if deviceCount >= effectiveDeviceLimit(&st, u) {
			return errors.New("device limit reached")
		}
		return nil
	}); err != nil {
		return VPNPeer{}, "", err
	}

	client := in.ClientAddress
	if client == "" {
		var err error
		client, err = nextWGAddress(in.ServerAddress, used)
		if err != nil {
			return VPNPeer{}, "", err
		}
	} else {
		var err error
		client, err = validateWGClientAddress(in.ServerAddress, client, used)
		if err != nil {
			return VPNPeer{}, "", err
		}
	}

	var srv map[string]any
	if err := s.agentJSON(n, http.MethodPost, "/v1/wireguard/server", map[string]any{
		"interface":          in.Interface,
		"address":            in.ServerAddress,
		"listen_port":        in.ListenPort,
		"internet_interface": n.InternetInterface,
	}, &srv); err != nil {
		return VPNPeer{}, "", err
	}

	endpoint := net.JoinHostPort(n.PublicIP, strconv.Itoa(in.ListenPort))
	var resp struct {
		PrivateKey      string `json:"private_key"`
		PublicKey       string `json:"public_key"`
		ServerPublicKey string `json:"server_public_key"`
		Config          string `json:"config"`
	}
	if err := s.agentJSON(n, http.MethodPost, "/v1/wireguard/peer", map[string]any{
		"interface":            in.Interface,
		"client_address":       client,
		"endpoint":             endpoint,
		"dns":                  in.DNS,
		"persistent_keepalive": 25,
	}, &resp); err != nil {
		return VPNPeer{}, "", err
	}

	priv, _ := s.crypt.Seal(resp.PrivateKey)
	cfg, _ := s.crypt.Seal(resp.Config)
	p := VPNPeer{
		ID:              randomHex(12),
		UserID:          userID,
		NodeID:          in.NodeID,
		DeviceName:      defaultString(in.DeviceName, "device-"+strconv.Itoa(deviceCount+1)),
		Interface:       in.Interface,
		Address:         client,
		PublicKey:       resp.PublicKey,
		PrivateKeyEnc:   priv,
		ConfigEnc:       cfg,
		ServerPublicKey: resp.ServerPublicKey,
		Endpoint:        endpoint,
		Enabled:         true,
		CreatedAt:       time.Now().UTC(),
	}
	_ = s.store.Update(func(st *State) error {
		st.VPNPeers = append(st.VPNPeers, p)
		return nil
	})
	return p, resp.Config, nil
}

func (s *Server) handleOnlineUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var out []map[string]any
	_ = s.store.Read(func(st State) error {
		for _, u := range st.Users {
			if !u.Online {
				continue
			}
			out = append(out, map[string]any{
				"id":                 u.ID,
				"username":           u.Username,
				"status":             u.Status,
				"online":             u.Online,
				"last_online_at":     u.LastOnlineAt,
				"traffic_bytes":      u.TrafficUsedBytes,
				"xray_bytes":         u.XrayTrafficBytes,
				"wireguard_bytes":    u.WireGuardTrafficBytes,
				"next_traffic_reset": u.NextTrafficResetAt,
			})
		}
		return nil
	})
	jsonWrite(w, http.StatusOK, out)
}

func (s *Server) handleTrafficSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var out []map[string]any
	_ = s.store.Read(func(st State) error {
		for _, u := range st.Users {
			out = append(out, map[string]any{
				"id":                  u.ID,
				"username":            u.Username,
				"status":              u.Status,
				"traffic_bytes":       u.TrafficUsedBytes,
				"xray_bytes":          u.XrayTrafficBytes,
				"wireguard_bytes":     u.WireGuardTrafficBytes,
				"limit_bytes":         effectiveUserLimit(&st, u),
				"reset_interval_days": effectiveResetIntervalDays(&st, u),
				"next_reset_at":       u.NextTrafficResetAt,
			})
		}
		return nil
	})
	jsonWrite(w, http.StatusOK, out)
}

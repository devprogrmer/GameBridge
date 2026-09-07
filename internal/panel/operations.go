// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type tunnelCreate struct {
	Name              string `json:"name"`
	SourceNodeID      string `json:"source_node_id"`
	DestinationNodeID string `json:"destination_node_id"`
	Transport         string `json:"transport"`
	Profile           string `json:"profile"`
	Ports             []int  `json:"ports"`
	MTU               int    `json:"mtu"`
	CIDR              string `json:"cidr"`
	Interface         string `json:"interface"`
	VNI               int    `json:"vni"`
}

func allowedTransport(t string) bool {
	switch t {
	case "pulseudp", "pulseudp-mp", "kcp", "quic", "icmp", "gre", "gre6", "ipip", "sit", "geneve", "vxlan":
		return true
	}
	return false
}
func nodeStatusAfterProbe(enabled, maintenance bool, failures int) string {
	if !enabled {
		return NodeStatusDisabled
	}
	if maintenance {
		return NodeStatusMaintenance
	}
	if failures >= 3 {
		return NodeStatusOffline
	}
	if failures > 0 {
		return NodeStatusDegraded
	}
	return NodeStatusOnline
}

func validateNodeForWork(n Node) error {
	if !n.Enabled {
		return fmt.Errorf("node %q is disabled", n.Name)
	}
	if n.Maintenance {
		return fmt.Errorf("node %q is in maintenance", n.Name)
	}
	if n.Status != NodeStatusOnline {
		return fmt.Errorf("node %q is not online (status: %s)", n.Name, n.Status)
	}
	return nil
}
func (s *Server) handleTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x []Tunnel
		_ = s.store.Read(func(st State) error { x = st.Tunnels; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		if requireRole(r, "operator") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in tunnelCreate
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		t, e := s.createTunnel(in)
		if e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		s.audit(r, "create", "tunnel:"+t.Name)
		jsonWrite(w, 201, t)
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) createTunnel(in tunnelCreate) (Tunnel, error) {
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	if !allowedTransport(in.Transport) {
		return Tunnel{}, errors.New("unsupported transport")
	}
	if in.Name == "" || strings.ContainsAny(in.Name, "/\\ \t\n") {
		return Tunnel{}, errors.New("invalid tunnel name")
	}
	if in.Profile == "" {
		in.Profile = "competitive"
	}
	if in.MTU == 0 {
		switch in.Transport {
		case "icmp":
			in.MTU = 900
		case "quic":
			in.MTU = 1280
		default:
			in.MTU = 1360
		}
	}
	if in.Interface == "" {
		in.Interface = "gb0"
	}
	if in.CIDR == "" {
		in.CIDR = "10.20.0.0/30"
	}
	var src, dst Node
	e := s.store.Read(func(st State) error {
		a := findNode(&st, in.SourceNodeID)
		b := findNode(&st, in.DestinationNodeID)
		if a == nil || b == nil {
			return errors.New("source or destination node not found")
		}
		src = *a
		dst = *b
		return nil
	})
	if e != nil {
		return Tunnel{}, e
	}
	if src.ID == dst.ID {
		return Tunnel{}, errors.New("source and destination node must be different")
	}
	if e := validateNodeForWork(src); e != nil {
		return Tunnel{}, fmt.Errorf("source node: %w", e)
	}
	if e := validateNodeForWork(dst); e != nil {
		return Tunnel{}, fmt.Errorf("destination node: %w", e)
	}
	srcCIDR, dstCIDR, srcIP, dstIP, e := derivePair(in.CIDR)
	if e != nil {
		return Tunnel{}, e
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	keyHex := hex.EncodeToString(key)
	kernel := map[string]bool{"gre": true, "gre6": true, "ipip": true, "sit": true, "geneve": true, "vxlan": true}[in.Transport]
	srcSpec := map[string]any{"name": in.Name, "kind": "userspace", "role": "client", "transport": in.Transport, "peer_host": dst.PublicIP, "ports": in.Ports, "interface": in.Interface, "local_cidr": srcCIDR, "peer_ip": dstIP, "mtu": in.MTU, "key_hex": keyHex, "profile": in.Profile, "nat": false, "internet_interface": "", "vni": in.VNI}
	dstSpec := map[string]any{"name": in.Name, "kind": "userspace", "role": "server", "transport": in.Transport, "peer_host": "", "ports": in.Ports, "interface": in.Interface, "local_cidr": dstCIDR, "peer_ip": srcIP, "mtu": in.MTU, "key_hex": keyHex, "profile": in.Profile, "nat": true, "internet_interface": dst.InternetInterface, "vni": in.VNI}
	if in.Transport == "pulseudp-mp" {
		dup := in.Profile == "stable" || in.Profile == "lossy"
		srcSpec["duplicate_small_packets"] = dup
		dstSpec["duplicate_small_packets"] = dup
	}
	if in.Transport == "kcp" {
		par := 0
		if in.Profile == "stable" {
			par = 2
		}
		if in.Profile == "lossy" {
			par = 3
		}
		for _, m := range []map[string]any{srcSpec, dstSpec} {
			m["kcp_data_shards"] = 10
			m["kcp_parity_shards"] = par
		}
	}
	if in.Transport == "icmp" {
		for _, m := range []map[string]any{srcSpec, dstSpec} {
			m["icmp_id"] = 4242
			m["icmp_poll_ms"] = 8
			m["icmp_burst"] = 4
		}
	}
	if kernel {
		vni := in.VNI
		if vni == 0 {
			vni = 100
		}
		srcSpec = map[string]any{"name": in.Name, "kind": "kernel", "transport": in.Transport, "interface": in.Interface, "local_public": src.PublicIP, "remote_public": dst.PublicIP, "local_cidr": srcCIDR, "remote_ip": dstIP, "mtu": in.MTU, "vni": vni}
		dstSpec = map[string]any{"name": in.Name, "kind": "kernel", "transport": in.Transport, "interface": in.Interface, "local_public": dst.PublicIP, "remote_public": src.PublicIP, "local_cidr": dstCIDR, "remote_ip": srcIP, "mtu": in.MTU, "vni": vni}
	}
	if e = s.agentJSON(dst, http.MethodPost, "/v1/tunnels", dstSpec, nil); e != nil {
		return Tunnel{}, fmt.Errorf("destination node: %w", e)
	}
	if e = s.agentJSON(src, http.MethodPost, "/v1/tunnels", srcSpec, nil); e != nil {
		_ = s.agentJSON(dst, http.MethodDelete, "/v1/tunnels/"+url.PathEscape(in.Name), nil, nil)
		return Tunnel{}, fmt.Errorf("source node: %w", e)
	}
	t := Tunnel{ID: randomHex(12), Name: in.Name, SourceNodeID: src.ID, DestinationNodeID: dst.ID, Transport: in.Transport, Profile: in.Profile, Ports: in.Ports, MTU: in.MTU, CIDR: in.CIDR, Interface: in.Interface, Status: "online", VNI: in.VNI, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_ = s.store.Update(func(st *State) error { st.Tunnels = append(st.Tunnels, t); return nil })
	return t, nil
}
func derivePair(cidr string) (string, string, string, string, error) {
	ip, n, e := net.ParseCIDR(cidr)
	if e != nil {
		return "", "", "", "", e
	}
	ones, _ := n.Mask.Size()
	a := append(net.IP(nil), ip...)
	b := append(net.IP(nil), ip...)
	incIP(a)
	incIP(b)
	incIP(b)
	if !n.Contains(a) || !n.Contains(b) {
		return "", "", "", "", errors.New("CIDR too small")
	}
	return fmt.Sprintf("%s/%d", a, ones), fmt.Sprintf("%s/%d", b, ones), a.String(), b.String(), nil
}
func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}
func (s *Server) handleTunnelItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) == 2 {
		if requireRole(r, "operator") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		a := parts[1]
		if a != "start" && a != "stop" && a != "restart" && a != "refresh" {
			jsonError(w, 400, "invalid action")
			return
		}
		if e := s.tunnelAction(id, a); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		s.audit(r, a, "tunnel:"+id)
		jsonWrite(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodDelete {
		if requireRole(r, "operator") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		if e := s.deleteTunnel(id); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		s.audit(r, "delete", "tunnel:"+id)
		jsonWrite(w, 200, map[string]bool{"ok": true})
		return
	}
	jsonError(w, 405, "method not allowed")
}
func (s *Server) tunnelAction(id, action string) error {
	var t Tunnel
	var src, dst Node
	if e := s.store.Read(func(st State) error {
		x := findTunnel(&st, id)
		if x == nil {
			return errors.New("tunnel not found")
		}
		t = *x
		a := findNode(&st, t.SourceNodeID)
		b := findNode(&st, t.DestinationNodeID)
		if a == nil || b == nil {
			return errors.New("node missing")
		}
		src = *a
		dst = *b
		return nil
	}); e != nil {
		return e
	}
	remoteAction := action
	if action == "refresh" {
		remoteAction = "status"
	}
	var x map[string]any
	e1 := s.agentJSON(src, http.MethodPost, "/v1/tunnels/"+url.PathEscape(t.Name)+"/"+remoteAction, nil, &x)
	e2 := s.agentJSON(dst, http.MethodPost, "/v1/tunnels/"+url.PathEscape(t.Name)+"/"+remoteAction, nil, &x)
	status := "online"
	if action == "stop" {
		status = "stopped"
	}
	if e1 != nil || e2 != nil {
		status = "degraded"
	}
	_ = s.store.Update(func(st *State) error {
		if q := findTunnel(st, id); q != nil {
			q.Status = status
			q.UpdatedAt = time.Now().UTC()
		}
		return nil
	})
	if e1 != nil {
		return e1
	}
	return e2
}
func (s *Server) deleteTunnel(id string) error {
	var t Tunnel
	var src, dst Node
	if e := s.store.Read(func(st State) error {
		x := findTunnel(&st, id)
		if x == nil {
			return errors.New("tunnel not found")
		}
		t = *x
		if a := findNode(&st, t.SourceNodeID); a != nil {
			src = *a
		}
		if b := findNode(&st, t.DestinationNodeID); b != nil {
			dst = *b
		}
		return nil
	}); e != nil {
		return e
	}
	if src.ID != "" {
		_ = s.agentJSON(src, http.MethodDelete, "/v1/tunnels/"+url.PathEscape(t.Name), nil, nil)
	}
	if dst.ID != "" {
		_ = s.agentJSON(dst, http.MethodDelete, "/v1/tunnels/"+url.PathEscape(t.Name), nil, nil)
	}
	return s.store.Update(func(st *State) error {
		st.Tunnels = deleteByID(st.Tunnels, id, func(x Tunnel) string { return x.ID })
		return nil
	})
}
func (s *Server) handleForwards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x []PortForward
		_ = s.store.Read(func(st State) error { x = st.Forwards; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		if requireRole(r, "operator") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in PortForward
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		if in.Protocol != "udp" && in.Protocol != "tcp" && in.Protocol != "both" {
			jsonError(w, 400, "protocol must be udp, tcp or both")
			return
		}
		var n Node
		if e := s.store.Read(func(st State) error {
			x := findNode(&st, in.NodeID)
			if x == nil {
				return errors.New("node not found")
			}
			n = *x
			return nil
		}); e != nil {
			jsonError(w, 404, e.Error())
			return
		}
		in.ID = randomHex(10)
		in.Enabled = true
		in.CreatedAt = time.Now().UTC()
		if e := s.agentJSON(n, http.MethodPost, "/v1/forwards", in, nil); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		_ = s.store.Update(func(st *State) error { st.Forwards = append(st.Forwards, in); return nil })
		s.audit(r, "create", "forward:"+in.ID)
		jsonWrite(w, 201, in)
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleForwardItem(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodDelete || requireRole(r, "operator") != nil {
		jsonError(w, 405, "method not allowed")
		return
	}
	var f PortForward
	var n Node
	_ = s.store.Read(func(st State) error {
		for _, x := range st.Forwards {
			if x.ID == id {
				f = x
			}
		}
		if x := findNode(&st, f.NodeID); x != nil {
			n = *x
		}
		return nil
	})
	if n.ID != "" {
		_ = s.agentJSON(n, http.MethodDelete, "/v1/forwards/"+url.PathEscape(id), nil, nil)
	}
	_ = s.store.Update(func(st *State) error {
		st.Forwards = deleteByID(st.Forwards, id, func(x PortForward) string { return x.ID })
		return nil
	})
	s.audit(r, "delete", "forward:"+id)
	jsonWrite(w, 200, map[string]bool{"ok": true})
}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	unit := r.URL.Query().Get("unit")
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "200"
	}
	var n Node
	if e := s.store.Read(func(st State) error {
		x := findNode(&st, nodeID)
		if x == nil {
			return errors.New("node not found")
		}
		n = *x
		return nil
	}); e != nil {
		jsonError(w, 404, e.Error())
		return
	}
	var out map[string]any
	if e := s.agentJSON(n, http.MethodGet, "/v1/logs?unit="+url.QueryEscape(unit)+"&lines="+url.QueryEscape(lines), nil, &out); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	jsonWrite(w, 200, out)
}
func (s *Server) agentJSON(n Node, method, path string, body, out any) error {
	tok, e := s.crypt.Open(n.AgentTokenEnc)
	if e != nil {
		return e
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, e := http.NewRequest(method, strings.TrimRight(n.AgentURL, "/")+path, rd)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := s.httpClient.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("agent HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
func (s *Server) probeNode(id string) {
	var n Node
	if s.store.Read(func(st State) error {
		x := findNode(&st, id)
		if x == nil {
			return errors.New("node not found")
		}
		n = *x
		return nil
	}) != nil {
		return
	}

	now := time.Now().UTC()
	if !n.Enabled || n.Maintenance {
		_ = s.store.Update(func(st *State) error {
			if x := findNode(st, id); x != nil {
				x.Status = nodeStatusAfterProbe(x.Enabled, x.Maintenance, x.FailureCount)
				x.UpdatedAt = now
			}
			return nil
		})
		return
	}

	var resp struct {
		Metrics NodeMetrics `json:"metrics"`
	}
	err := s.agentJSON(n, http.MethodGet, "/v1/status", nil, &resp)

	_ = s.store.Update(func(st *State) error {
		x := findNode(st, id)
		if x == nil {
			return nil
		}
		x.UpdatedAt = now
		if err != nil {
			x.FailureCount++
			x.LastError = err.Error()
			x.Status = nodeStatusAfterProbe(x.Enabled, x.Maintenance, x.FailureCount)
			return nil
		}
		x.Status = NodeStatusOnline
		x.LastSeen = now
		x.Metrics = resp.Metrics
		x.FailureCount = 0
		x.LastError = ""
		return nil
	})
}
func (s *Server) background() {
	time.Sleep(2 * time.Second)
	probe := time.NewTicker(15 * time.Second)
	sync := time.NewTicker(60 * time.Second)
	defer probe.Stop()
	defer sync.Stop()
	for {
		select {
		case <-probe.C:
			var ids []string
			_ = s.store.Read(func(st State) error {
				for _, n := range st.Nodes {
					if n.Enabled {
						ids = append(ids, n.ID)
					}
				}
				return nil
			})
			for _, id := range ids {
				go s.probeNode(id)
			}
		case <-sync.C:
			s.syncWireGuard()
		}
	}
}

type wgCreate struct {
	NodeID        string `json:"node_id"`
	DeviceName    string `json:"device_name"`
	Interface     string `json:"interface"`
	ServerAddress string `json:"server_address"`
	ClientAddress string `json:"client_address"`
	DNS           string `json:"dns"`
	ListenPort    int    `json:"listen_port"`
}

func (s *Server) handleUserWireGuard(w http.ResponseWriter, r *http.Request, userID string) {
	switch r.Method {
	case http.MethodGet:
		var out []map[string]any
		_ = s.store.Read(func(st State) error {
			for _, p := range st.VPNPeers {
				if p.UserID == userID {
					cfg, _ := s.crypt.Open(p.ConfigEnc)
					out = append(out, map[string]any{"peer": p, "config": cfg})
				}
			}
			return nil
		})
		jsonWrite(w, 200, out)
	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in wgCreate
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		if in.Interface == "" {
			in.Interface = "gbwg0"
		}
		if in.ServerAddress == "" {
			in.ServerAddress = "10.77.0.1/24"
		}
		if in.ListenPort == 0 {
			in.ListenPort = 51820
		}
		if in.DNS == "" {
			in.DNS = "1.1.1.1"
		}
		p, c, e := s.createWGPeer(userID, in)
		if e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		s.audit(r, "create", "wireguard-peer:"+p.ID)
		jsonWrite(w, 201, map[string]any{"peer": p, "config": c})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) createWGPeerLegacy(userID string, in wgCreate) (VPNPeer, string, error) {
	var u User
	var n Node
	count := 0
	if e := s.store.Read(func(st State) error {
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
				count++
			}
		}
		if count >= effectiveDeviceLimit(&st, u) {
			return errors.New("device limit reached")
		}
		return nil
	}); e != nil {
		return VPNPeer{}, "", e
	}
	client := in.ClientAddress
	if client == "" {
		ip, network, e := net.ParseCIDR(in.ServerAddress)
		if e != nil {
			return VPNPeer{}, "", e
		}
		candidate := append(net.IP(nil), ip...)
		for i := 0; i < count+1; i++ {
			incIP(candidate)
		}
		if !network.Contains(candidate) {
			return VPNPeer{}, "", errors.New("WireGuard subnet exhausted")
		}
		client = candidate.String() + "/32"
	}
	var srv map[string]any
	if e := s.agentJSON(n, http.MethodPost, "/v1/wireguard/server", map[string]any{"interface": in.Interface, "address": in.ServerAddress, "listen_port": in.ListenPort, "internet_interface": n.InternetInterface}, &srv); e != nil {
		return VPNPeer{}, "", e
	}
	endpoint := net.JoinHostPort(n.PublicIP, strconv.Itoa(in.ListenPort))
	var resp struct {
		PrivateKey      string `json:"private_key"`
		PublicKey       string `json:"public_key"`
		ServerPublicKey string `json:"server_public_key"`
		Config          string `json:"config"`
	}
	if e := s.agentJSON(n, http.MethodPost, "/v1/wireguard/peer", map[string]any{"interface": in.Interface, "client_address": client, "endpoint": endpoint, "dns": in.DNS, "persistent_keepalive": 25}, &resp); e != nil {
		return VPNPeer{}, "", e
	}
	priv, _ := s.crypt.Seal(resp.PrivateKey)
	cfg, _ := s.crypt.Seal(resp.Config)
	p := VPNPeer{ID: randomHex(12), UserID: userID, NodeID: in.NodeID, DeviceName: defaultString(in.DeviceName, "device-"+strconv.Itoa(count+1)), Interface: in.Interface, Address: client, PublicKey: resp.PublicKey, PrivateKeyEnc: priv, ConfigEnc: cfg, ServerPublicKey: resp.ServerPublicKey, Endpoint: endpoint, Enabled: true, CreatedAt: time.Now().UTC()}
	_ = s.store.Update(func(st *State) error { st.VPNPeers = append(st.VPNPeers, p); return nil })
	return p, resp.Config, nil
}
func (s *Server) deleteRemotePeer(p VPNPeer) error {
	var n Node
	if e := s.store.Read(func(st State) error {
		x := findNode(&st, p.NodeID)
		if x == nil {
			return errors.New("node missing")
		}
		n = *x
		return nil
	}); e != nil {
		return e
	}
	return s.agentJSON(n, http.MethodDelete, "/v1/wireguard/peer?interface="+url.QueryEscape(p.Interface)+"&public_key="+url.QueryEscape(p.PublicKey), nil, nil)
}
func (s *Server) syncWireGuardLegacy() {
	var nodes []Node
	_ = s.store.Read(func(st State) error { nodes = st.Nodes; return nil })
	type stat struct {
		Interface string `json:"interface"`
		PublicKey string `json:"public_key"`
		RXBytes   int64  `json:"rx_bytes"`
		TXBytes   int64  `json:"tx_bytes"`
	}
	seen := map[string]stat{}
	for _, n := range nodes {
		if !n.Enabled {
			continue
		}
		var resp struct {
			Peers []stat `json:"peers"`
		}
		if s.agentJSON(n, http.MethodGet, "/v1/wireguard/stats", nil, &resp) == nil {
			for _, p := range resp.Peers {
				seen[n.ID+"|"+p.PublicKey] = p
			}
		}
	}
	var disable []VPNPeer
	_ = s.store.Update(func(st *State) error {
		now := time.Now().UTC()
		for i := range st.VPNPeers {
			p := &st.VPNPeers[i]
			if x, ok := seen[p.NodeID+"|"+p.PublicKey]; ok {
				p.RXBytes = x.RXBytes
				p.TXBytes = x.TXBytes
			}
		}
		for i := range st.Users {
			u := &st.Users[i]
			var total int64
			for _, p := range st.VPNPeers {
				if p.UserID == u.ID {
					total += p.RXBytes + p.TXBytes
				}
			}
			u.TrafficUsedBytes = total
			limit := effectiveUserLimit(st, *u)
			expired := !u.ExpiresAt.IsZero() && now.After(u.ExpiresAt)
			if u.Status == "active" && ((limit > 0 && total >= limit) || expired) {
				if expired {
					u.Status = "expired"
				} else {
					u.Status = "quota-exceeded"
				}
				for _, p := range st.VPNPeers {
					if p.UserID == u.ID && p.Enabled {
						disable = append(disable, p)
					}
				}
			}
		}
		return nil
	})
	for _, p := range disable {
		_ = s.deleteRemotePeer(p)
		_ = s.store.Update(func(st *State) error {
			if x := findPeer(st, p.ID); x != nil {
				x.Enabled = false
			}
			return nil
		})
	}
}

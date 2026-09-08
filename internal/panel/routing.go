// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var xrayTagPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)
var wireGuardInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

func defaultWireGuardInterface(tag string) string {
	name := "gbw-" + tag
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

func defaultOpenVPNInterface(tag string) string {
	name := "gbv-" + tag
	if len(name) > 15 {
		name = name[:15]
	}
	return name
}

func defaultOpenVPNRoutingID(tag string) int {
	h := 5381
	for _, b := range []byte(tag) {
		h = ((h << 5) + h + int(b)) % 50000
	}
	return 10000 + h
}

type outboundInput struct {
	Name                   string   `json:"name"`
	NodeID                 string   `json:"node_id"`
	Tag                    string   `json:"tag"`
	Protocol               string   `json:"protocol"`
	Address                string   `json:"address"`
	Port                   int      `json:"port"`
	Username               string   `json:"username"`
	Password               string   `json:"password"`
	Secret                 string   `json:"secret"`
	Transport              string   `json:"transport"`
	TLSMode                string   `json:"tls_mode"`
	Path                   string   `json:"path"`
	Host                   string   `json:"host"`
	ServiceName            string   `json:"service_name"`
	ServerName             string   `json:"server_name"`
	AllowInsecure          bool     `json:"allow_insecure"`
	Fingerprint            string   `json:"fingerprint"`
	Flow                   string   `json:"flow"`
	ShadowsocksMethod      string   `json:"shadowsocks_method"`
	RealityPublicKey       string   `json:"reality_public_key"`
	RealityShortID         string   `json:"reality_short_id"`
	WireGuardInterface     string   `json:"wireguard_interface"`
	WireGuardAddress       string   `json:"wireguard_address"`
	WireGuardPeerPublicKey string   `json:"wireguard_peer_public_key"`
	WireGuardAllowedIPs    []string `json:"wireguard_allowed_ips"`
	WireGuardKeepalive     int      `json:"wireguard_keepalive"`
	WireGuardMTU           int      `json:"wireguard_mtu"`
	TorSOCKSPort           int      `json:"tor_socks_port"`
	OpenVPNInterface       string   `json:"openvpn_interface"`
	OpenVPNRoutingTable    int      `json:"openvpn_routing_table"`
	OpenVPNMark            int      `json:"openvpn_mark"`
	CustomXrayProtocol     string   `json:"custom_xray_protocol"`
	Enabled                bool     `json:"enabled"`
	Remark                 string   `json:"remark"`
}

func outboundInputSecret(in outboundInput) string {
	if strings.TrimSpace(in.Secret) != "" {
		return strings.TrimSpace(in.Secret)
	}
	return strings.TrimSpace(in.Password)
}

func outboundProtocolNeedsSecret(protocol string) bool {
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "wireguard", "openvpn", "custom":
		return true
	default:
		return false
	}
}

func normalizeOutboundInput(in *outboundInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.Tag = strings.TrimSpace(in.Tag)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Address = strings.TrimSpace(in.Address)
	in.Username = strings.TrimSpace(in.Username)
	in.Password = strings.TrimSpace(in.Password)
	in.Secret = strings.TrimSpace(in.Secret)
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	in.TLSMode = strings.ToLower(strings.TrimSpace(in.TLSMode))
	in.Path = strings.TrimSpace(in.Path)
	in.Host = strings.TrimSpace(in.Host)
	in.ServiceName = strings.TrimSpace(in.ServiceName)
	in.ServerName = strings.TrimSpace(in.ServerName)
	in.Fingerprint = strings.TrimSpace(in.Fingerprint)
	in.Flow = strings.TrimSpace(in.Flow)
	in.ShadowsocksMethod = strings.ToLower(strings.TrimSpace(in.ShadowsocksMethod))
	in.RealityPublicKey = strings.TrimSpace(in.RealityPublicKey)
	in.RealityShortID = strings.TrimSpace(in.RealityShortID)
	in.WireGuardInterface = strings.TrimSpace(in.WireGuardInterface)
	in.WireGuardAddress = strings.TrimSpace(in.WireGuardAddress)
	in.WireGuardPeerPublicKey = strings.TrimSpace(in.WireGuardPeerPublicKey)
	in.OpenVPNInterface = strings.TrimSpace(in.OpenVPNInterface)
	in.CustomXrayProtocol = strings.TrimSpace(in.CustomXrayProtocol)
	if in.Protocol != "custom" {
		in.CustomXrayProtocol = ""
	}
	for i := range in.WireGuardAllowedIPs {
		in.WireGuardAllowedIPs[i] = strings.TrimSpace(in.WireGuardAllowedIPs[i])
	}
	in.Remark = strings.TrimSpace(in.Remark)

	if in.Name == "" || in.NodeID == "" {
		return errors.New("name and node_id are required")
	}
	if !xrayTagPattern.MatchString(in.Tag) || in.Tag == "direct" || in.Tag == "blocked" || in.Tag == "api" {
		return errors.New("invalid or reserved outbound tag")
	}

	switch in.Protocol {
	case "freedom", "blackhole":
		in.Address = ""
		in.Port = 0
		in.Username = ""
		in.Password = ""
		in.Secret = ""
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		return nil

	case "socks", "http":
		if in.Address == "" || in.Port < 1 || in.Port > 65535 {
			return errors.New("proxy outbound requires address and valid port")
		}
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		return nil

	case "vless", "vmess", "trojan":
		if in.Address == "" || in.Port < 1 || in.Port > 65535 {
			return errors.New("xray proxy outbound requires address and valid port")
		}
		if in.Transport == "" {
			in.Transport = "raw"
		}
		switch in.Transport {
		case "raw", "ws", "grpc":
		default:
			return errors.New("transport must be raw, ws or grpc")
		}
		if in.TLSMode == "" {
			in.TLSMode = "none"
		}
		switch in.TLSMode {
		case "none", "tls", "reality":
		default:
			return errors.New("tls_mode must be none, tls or reality")
		}
		if in.Transport == "ws" && in.TLSMode == "reality" {
			return errors.New("REALITY does not support websocket transport")
		}
		if in.Transport == "ws" && in.Path == "" {
			in.Path = "/"
		}
		if in.Transport == "grpc" && in.ServiceName == "" {
			in.ServiceName = "gamebridge"
		}
		if in.TLSMode == "tls" || in.TLSMode == "reality" {
			if in.ServerName == "" {
				in.ServerName = in.Address
			}
			if in.Fingerprint == "" {
				in.Fingerprint = "chrome"
			}
		}
		if in.TLSMode == "reality" && in.RealityPublicKey == "" {
			return errors.New("reality_public_key is required for REALITY")
		}
		if in.Protocol != "vless" {
			in.Flow = ""
		} else if in.Flow != "" && in.Flow != "xtls-rprx-vision" {
			return errors.New("unsupported VLESS flow")
		} else if in.Flow != "" && (in.Transport != "raw" || (in.TLSMode != "tls" && in.TLSMode != "reality")) {
			return errors.New("VLESS Vision flow requires raw transport with TLS or REALITY")
		}
		in.ShadowsocksMethod = ""
		return nil

	case "wireguard":
		if in.Address == "" || in.Port < 1 || in.Port > 65535 {
			return errors.New("wireguard outbound requires endpoint address and valid port")
		}
		if in.WireGuardInterface == "" {
			in.WireGuardInterface = defaultWireGuardInterface(in.Tag)
		}
		if !wireGuardInterfacePattern.MatchString(in.WireGuardInterface) {
			return errors.New("wireguard_interface must be 1-15 safe interface characters")
		}
		if _, _, err := net.ParseCIDR(in.WireGuardAddress); err != nil {
			return errors.New("wireguard_address must be a valid CIDR")
		}
		if in.WireGuardPeerPublicKey == "" {
			return errors.New("wireguard_peer_public_key is required")
		}
		if len(in.WireGuardAllowedIPs) == 0 {
			in.WireGuardAllowedIPs = []string{"0.0.0.0/0", "::/0"}
		}
		for _, cidr := range in.WireGuardAllowedIPs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid wireguard_allowed_ips entry %q", cidr)
			}
		}
		if in.WireGuardKeepalive == 0 {
			in.WireGuardKeepalive = 25
		}
		if in.WireGuardKeepalive < 0 || in.WireGuardKeepalive > 65535 {
			return errors.New("wireguard_keepalive must be between 0 and 65535")
		}
		if in.WireGuardMTU == 0 {
			in.WireGuardMTU = 1420
		}
		if in.WireGuardMTU < 576 || in.WireGuardMTU > 9000 {
			return errors.New("wireguard_mtu must be between 576 and 9000")
		}
		in.Username = ""
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		in.TorSOCKSPort = 0
		return nil

	case "tor":
		if in.TorSOCKSPort == 0 {
			in.TorSOCKSPort = 19050
		}
		if in.TorSOCKSPort < 1024 || in.TorSOCKSPort > 65535 {
			return errors.New("tor_socks_port must be between 1024 and 65535")
		}
		in.Address = ""
		in.Port = 0
		in.Username = ""
		in.Password = ""
		in.Secret = ""
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		in.WireGuardInterface = ""
		in.WireGuardAddress = ""
		in.WireGuardPeerPublicKey = ""
		in.WireGuardAllowedIPs = nil
		in.WireGuardKeepalive = 0
		in.WireGuardMTU = 0
		in.OpenVPNInterface = ""
		in.OpenVPNRoutingTable = 0
		in.OpenVPNMark = 0
		return nil

	case "openvpn":
		if in.OpenVPNInterface == "" {
			in.OpenVPNInterface = defaultOpenVPNInterface(in.Tag)
		}
		if !wireGuardInterfacePattern.MatchString(in.OpenVPNInterface) {
			return errors.New("openvpn_interface must be 1-15 safe interface characters")
		}
		if in.OpenVPNRoutingTable == 0 {
			in.OpenVPNRoutingTable = defaultOpenVPNRoutingID(in.Tag)
		}
		if in.OpenVPNMark == 0 {
			in.OpenVPNMark = in.OpenVPNRoutingTable
		}
		if in.OpenVPNRoutingTable < 1000 || in.OpenVPNRoutingTable > 65000 {
			return errors.New("openvpn_routing_table must be between 1000 and 65000")
		}
		if in.OpenVPNMark < 1000 || in.OpenVPNMark > 65000 {
			return errors.New("openvpn_mark must be between 1000 and 65000")
		}
		in.Address = ""
		in.Port = 0
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		in.WireGuardInterface = ""
		in.WireGuardAddress = ""
		in.WireGuardPeerPublicKey = ""
		in.WireGuardAllowedIPs = nil
		in.WireGuardKeepalive = 0
		in.WireGuardMTU = 0
		in.TorSOCKSPort = 0
		return nil

	case "custom":
		if in.Password != "" {
			return errors.New("custom xray config must be supplied in secret, not password")
		}
		in.Username = ""
		if in.Secret != "" {
			_, protocol, canonical, err := decodeCustomXrayConfig(in.Secret)
			if err != nil {
				return err
			}
			in.Secret = canonical
			in.CustomXrayProtocol = protocol
		} else {
			in.CustomXrayProtocol = ""
		}
		in.Address = ""
		in.Port = 0
		in.Transport = ""
		in.TLSMode = ""
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.ShadowsocksMethod = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		in.WireGuardInterface = ""
		in.WireGuardAddress = ""
		in.WireGuardPeerPublicKey = ""
		in.WireGuardAllowedIPs = nil
		in.WireGuardKeepalive = 0
		in.WireGuardMTU = 0
		in.TorSOCKSPort = 0
		in.OpenVPNInterface = ""
		in.OpenVPNRoutingTable = 0
		in.OpenVPNMark = 0
		return nil

	case "shadowsocks":
		if in.Address == "" || in.Port < 1 || in.Port > 65535 {
			return errors.New("shadowsocks outbound requires address and valid port")
		}
		if in.ShadowsocksMethod == "" {
			in.ShadowsocksMethod = "aes-128-gcm"
		}
		if in.Transport == "" {
			in.Transport = "raw"
		}
		if in.Transport != "raw" {
			return errors.New("shadowsocks outbound currently supports raw transport only")
		}
		in.TLSMode = "none"
		in.Path = ""
		in.Host = ""
		in.ServiceName = ""
		in.ServerName = ""
		in.AllowInsecure = false
		in.Fingerprint = ""
		in.Flow = ""
		in.RealityPublicKey = ""
		in.RealityShortID = ""
		return nil

	default:
		return errors.New("protocol must be freedom, blackhole, socks, http, vless, vmess, trojan, shadowsocks, wireguard, tor, openvpn or custom")
	}
}
func normalizeRoutingRule(in *RoutingRule) error {
	in.Name = strings.TrimSpace(in.Name)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.Ports = strings.TrimSpace(in.Ports)
	in.Network = strings.ToLower(strings.TrimSpace(in.Network))
	in.OutboundID = strings.TrimSpace(in.OutboundID)
	in.OutboundGroupID = strings.TrimSpace(in.OutboundGroupID)
	if in.Name == "" || in.NodeID == "" {
		return errors.New("name and node_id are required")
	}
	if (in.OutboundID == "") == (in.OutboundGroupID == "") {
		return errors.New("exactly one of outbound_id or outbound_group_id is required")
	}
	if in.Network != "" && in.Network != "tcp" && in.Network != "udp" && in.Network != "tcp,udp" {
		return errors.New("network must be tcp, udp or tcp,udp")
	}
	for i := range in.Domains {
		in.Domains[i] = strings.TrimSpace(in.Domains[i])
	}
	for i := range in.IPs {
		in.IPs[i] = strings.TrimSpace(in.IPs[i])
	}
	for i := range in.Protocols {
		in.Protocols[i] = strings.ToLower(strings.TrimSpace(in.Protocols[i]))
	}
	return nil
}

func (s *Server) handleRoutingRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var out []RoutingRule
		_ = s.store.Read(func(st State) error {
			out = st.RoutingRules
			sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
			return nil
		})
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in RoutingRule
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeRoutingRule(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.ID = randomHex(12)
		in.Enabled = true
		in.CreatedAt = time.Now().UTC()
		in.UpdatedAt = in.CreatedAt
		if err := s.store.Update(func(st *State) error {
			if err := validateRoutingReferences(st, in); err != nil {
				return err
			}
			st.RoutingRules = append(st.RoutingRules, in)
			return nil
		}); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		deployErr := s.deployXrayNode(in.NodeID)
		s.audit(r, "create", "routing:"+in.Name)
		if deployErr != nil {
			jsonWrite(w, http.StatusCreated, map[string]any{"rule": in, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusCreated, map[string]any{"rule": in})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleRoutingRuleItem(w http.ResponseWriter, r *http.Request, id string) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	switch r.Method {
	case http.MethodGet:
		var out *RoutingRule
		_ = s.store.Read(func(st State) error {
			if x := findRoutingRule(&st, id); x != nil {
				cp := *x
				out = &cp
			}
			return nil
		})
		if out == nil {
			jsonError(w, http.StatusNotFound, "routing rule not found")
			return
		}
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPut:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in RoutingRule
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeRoutingRule(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		var oldNodeID string
		err := s.store.Update(func(st *State) error {
			x := findRoutingRule(st, id)
			if x == nil {
				return errors.New("routing rule not found")
			}
			oldNodeID = x.NodeID
			in.ID = x.ID
			in.CreatedAt = x.CreatedAt
			in.UpdatedAt = time.Now().UTC()
			if err := validateRoutingReferences(st, in); err != nil {
				return err
			}
			*x = in
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if oldNodeID != "" && oldNodeID != in.NodeID {
			_ = s.deployXrayNode(oldNodeID)
		}
		if err := s.deployXrayNode(in.NodeID); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, "update", "routing:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var nodeID string
		err := s.store.Update(func(st *State) error {
			x := findRoutingRule(st, id)
			if x == nil {
				return errors.New("routing rule not found")
			}
			nodeID = x.NodeID
			st.RoutingRules = deleteByID(st.RoutingRules, id, func(x RoutingRule) string { return x.ID })
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		deployErr := s.deployXrayNode(nodeID)
		s.audit(r, "delete", "routing:"+id)
		if deployErr != nil {
			jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func validateRoutingReferences(st *State, r RoutingRule) error {
	if findNode(st, r.NodeID) == nil {
		return errors.New("node not found")
	}
	if r.OutboundGroupID != "" {
		group := findOutboundGroup(st, r.OutboundGroupID)
		if group == nil || group.NodeID != r.NodeID || !group.Enabled {
			return errors.New("outbound group not found or not enabled on selected node")
		}
	} else if r.OutboundID != "direct" && r.OutboundID != "blocked" {
		out := findOutbound(st, r.OutboundID)
		if out == nil || out.NodeID != r.NodeID || !out.Enabled {
			return errors.New("outbound not found or not enabled on selected node")
		}
	}
	for _, id := range r.InboundIDs {
		in := findInbound(st, id)
		if in == nil || in.NodeID != r.NodeID {
			return errors.New("routing inbound does not belong to selected node")
		}
	}
	for _, id := range r.UserIDs {
		if findUser(st, id) == nil {
			return errors.New("routing user not found")
		}
	}
	return nil
}

func buildXrayOutboundStream(x Outbound) map[string]any {
	if x.Transport == "" {
		return nil
	}
	method := x.Transport
	switch method {
	case "ws":
		method = "websocket"
	case "raw":
		method = "raw"
	case "grpc":
		method = "grpc"
	}
	stream := map[string]any{"method": method}

	switch x.Transport {
	case "ws":
		ws := map[string]any{"path": defaultString(x.Path, "/")}
		if x.Host != "" {
			ws["host"] = x.Host
		}
		stream["wsSettings"] = ws
	case "grpc":
		stream["grpcSettings"] = map[string]any{
			"serviceName": defaultString(x.ServiceName, "gamebridge"),
		}
	}

	switch x.TLSMode {
	case "tls":
		tls := map[string]any{
			"serverName":    defaultString(x.ServerName, x.Address),
			"allowInsecure": x.AllowInsecure,
		}
		if x.Fingerprint != "" {
			tls["fingerprint"] = x.Fingerprint
		}
		stream["security"] = "tls"
		stream["tlsSettings"] = tls
	case "reality":
		reality := map[string]any{
			"serverName":  defaultString(x.ServerName, x.Address),
			"fingerprint": defaultString(x.Fingerprint, "chrome"),
			"password":    x.RealityPublicKey,
		}
		if x.RealityShortID != "" {
			reality["shortId"] = x.RealityShortID
		}
		stream["security"] = "reality"
		stream["realitySettings"] = reality
	default:
		stream["security"] = "none"
	}
	return stream
}

func (s *Server) buildXrayOutbounds(st State, nodeID string) ([]any, error) {
	out := []any{
		map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{}},
		map[string]any{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}},
	}
	for _, x := range st.Outbounds {
		if x.NodeID != nodeID || !x.Enabled {
			continue
		}
		item := map[string]any{"tag": x.Tag, "protocol": x.Protocol, "settings": map[string]any{}}

		var secret string
		if x.PasswordEnc != "" {
			var err error
			secret, err = s.crypt.Open(x.PasswordEnc)
			if err != nil {
				return nil, fmt.Errorf("decrypt outbound %s credential: %w", x.Tag, err)
			}
		}

		switch x.Protocol {
		case "freedom", "blackhole":

		case "socks", "http":
			settings := map[string]any{"address": x.Address, "port": x.Port}
			if x.Username != "" {
				settings["user"] = x.Username
				if secret != "" {
					settings["pass"] = secret
				}
			}
			item["settings"] = settings

		case "vless":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a VLESS credential", x.Tag)
			}
			settings := map[string]any{
				"address":    x.Address,
				"port":       x.Port,
				"id":         secret,
				"encryption": "none",
			}
			if x.Flow != "" {
				settings["flow"] = x.Flow
			}
			item["settings"] = settings
			if stream := buildXrayOutboundStream(x); stream != nil {
				item["streamSettings"] = stream
			}

		case "vmess":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a VMess credential", x.Tag)
			}
			item["settings"] = map[string]any{
				"address":  x.Address,
				"port":     x.Port,
				"id":       secret,
				"security": "auto",
			}
			if stream := buildXrayOutboundStream(x); stream != nil {
				item["streamSettings"] = stream
			}

		case "trojan":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a Trojan password", x.Tag)
			}
			item["settings"] = map[string]any{
				"address":  x.Address,
				"port":     x.Port,
				"password": secret,
			}
			if stream := buildXrayOutboundStream(x); stream != nil {
				item["streamSettings"] = stream
			}

		case "wireguard":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a WireGuard private key", x.Tag)
			}
			item["protocol"] = "freedom"
			item["settings"] = map[string]any{"domainStrategy": "AsIs"}
			item["streamSettings"] = map[string]any{
				"sockopt": map[string]any{
					"interface": x.WireGuardInterface,
				},
			}

		case "tor":
			if x.TorSOCKSPort < 1024 || x.TorSOCKSPort > 65535 {
				return nil, fmt.Errorf("outbound %s has invalid Tor SOCKS port", x.Tag)
			}
			item["protocol"] = "socks"
			item["settings"] = map[string]any{
				"address": "127.0.0.1",
				"port":    x.TorSOCKSPort,
			}

		case "openvpn":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires an openvpn profile", x.Tag)
			}
			if x.OpenVPNInterface == "" || x.OpenVPNMark < 1000 {
				return nil, fmt.Errorf("outbound %s has invalid openvpn routing settings", x.Tag)
			}
			item["protocol"] = "freedom"
			item["settings"] = map[string]any{"domainStrategy": "AsIs"}
			item["streamSettings"] = map[string]any{
				"sockopt": map[string]any{
					"interface": x.OpenVPNInterface,
					"mark":      x.OpenVPNMark,
				},
			}

		case "custom":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a custom xray config", x.Tag)
			}
			customItem, protocol, err := customXrayOutboundItem(x.Tag, secret)
			if err != nil {
				return nil, fmt.Errorf("outbound %s custom xray config: %w", x.Tag, err)
			}
			if x.CustomXrayProtocol != "" && x.CustomXrayProtocol != protocol {
				return nil, fmt.Errorf("outbound %s custom protocol metadata mismatch", x.Tag)
			}
			item = customItem

		case "shadowsocks":
			if secret == "" {
				return nil, fmt.Errorf("outbound %s requires a Shadowsocks password", x.Tag)
			}
			item["settings"] = map[string]any{
				"address":  x.Address,
				"port":     x.Port,
				"method":   defaultString(x.ShadowsocksMethod, "aes-128-gcm"),
				"password": secret,
			}
			if stream := buildXrayOutboundStream(x); stream != nil {
				item["streamSettings"] = stream
			}

		default:
			return nil, fmt.Errorf("unsupported outbound protocol %q", x.Protocol)
		}
		out = append(out, item)
	}

	aliases, err := buildXrayBalancerAliases(st, nodeID, out)
	if err != nil {
		return nil, fmt.Errorf("build outbound balancer aliases: %w", err)
	}
	out = append(out, aliases...)
	return out, nil
}
func buildXrayRouting(st State, nodeID string) map[string]any {
	rules := append([]RoutingRule(nil), st.RoutingRules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })
	var out []any
	for _, r := range rules {
		if r.NodeID != nodeID || !r.Enabled {
			continue
		}
		rule := map[string]any{
			"type":    "field",
			"ruleTag": "gb-route-" + r.ID,
		}
		if r.OutboundGroupID != "" {
			group := findOutboundGroup(&st, r.OutboundGroupID)
			if group == nil || !group.Enabled || group.NodeID != nodeID {
				continue
			}
			rule["balancerTag"] = outboundGroupBalancerTag(group.ID)
		} else {
			target := r.OutboundID
			if target != "direct" && target != "blocked" {
				if x := findOutbound(&st, target); x != nil && x.Enabled {
					target = x.Tag
				} else {
					continue
				}
			}
			rule["outboundTag"] = target
		}
		if len(r.InboundIDs) > 0 {
			tags := make([]string, 0, len(r.InboundIDs))
			for _, id := range r.InboundIDs {
				tags = append(tags, "gb-in-"+id)
			}
			rule["inboundTag"] = tags
		}
		if len(r.UserIDs) > 0 {
			users := make([]string, 0, len(r.UserIDs))
			for _, id := range r.UserIDs {
				if u := findUser(&st, id); u != nil {
					users = append(users, "gb:"+u.Username)
				}
			}
			if len(users) > 0 {
				rule["user"] = users
			}
		}
		if len(r.Domains) > 0 {
			rule["domain"] = r.Domains
		}
		if len(r.IPs) > 0 {
			rule["ip"] = r.IPs
		}
		if r.Ports != "" {
			rule["port"] = r.Ports
		}
		if r.Network != "" {
			rule["network"] = r.Network
		}
		if len(r.Protocols) > 0 {
			rule["protocol"] = r.Protocols
		}
		out = append(out, rule)
	}
	return map[string]any{"domainStrategy": "AsIs", "rules": out, "balancers": buildXrayBalancers(st, nodeID)}
}

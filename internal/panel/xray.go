// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type xrayApplyRequest struct {
	Config map[string]any `json:"config"`
}

type realityKeyResponse struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

func findInboundClient(st *State, inboundID, userID string) *InboundClient {
	for i := range st.InboundClients {
		if st.InboundClients[i].InboundID == inboundID && st.InboundClients[i].UserID == userID {
			return &st.InboundClients[i]
		}
	}
	return nil
}

func uuidV4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func randomCredential(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func credentialForProtocol(protocol string) string {
	switch protocol {
	case "vless", "vmess":
		return uuidV4()
	default:
		return randomCredential(24)
	}
}

func (s *Server) ensureRealityKeys(nodeID string) error {
	var node Node
	var targets []string

	if err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n
		for _, in := range st.Inbounds {
			if in.NodeID == nodeID && in.Enabled && in.TLSMode == "reality" && in.RealityPrivateKeyEnc == "" {
				targets = append(targets, in.ID)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, id := range targets {
		var keys realityKeyResponse
		if err := s.agentJSON(node, http.MethodPost, "/v1/xray/x25519", nil, &keys); err != nil {
			return fmt.Errorf("generate REALITY key: %w", err)
		}
		if keys.PrivateKey == "" || keys.PublicKey == "" {
			return errors.New("agent returned incomplete REALITY keypair")
		}
		enc, err := s.crypt.Seal(keys.PrivateKey)
		if err != nil {
			return err
		}
		if err := s.store.Update(func(st *State) error {
			in := findInbound(st, id)
			if in == nil {
				return errors.New("inbound disappeared while generating REALITY keys")
			}
			in.RealityPrivateKeyEnc = enc
			in.RealityPublicKey = keys.PublicKey
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) ensureShadowsocksServerPasswords(nodeID string) error {
	var ids []string
	if err := s.store.Read(func(st State) error {
		if findNode(&st, nodeID) == nil {
			return errors.New("node not found")
		}
		for _, in := range st.Inbounds {
			if in.NodeID == nodeID && in.Enabled && in.Protocol == "shadowsocks" && in.ShadowsocksServerPasswordEnc == "" {
				ids = append(ids, in.ID)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	for _, id := range ids {
		enc, err := s.crypt.Seal(randomCredential(32))
		if err != nil {
			return err
		}
		if err := s.store.Update(func(st *State) error {
			in := findInbound(st, id)
			if in == nil {
				return errors.New("inbound disappeared while generating Shadowsocks server password")
			}
			if in.ShadowsocksServerPasswordEnc == "" {
				in.ShadowsocksServerPasswordEnc = enc
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) buildXrayConfig(nodeID string) (map[string]any, Node, error) {
	var st State
	if err := s.store.Read(func(x State) error {
		st = x
		return nil
	}); err != nil {
		return nil, Node{}, err
	}

	node := findNode(&st, nodeID)
	if node == nil {
		return nil, Node{}, errors.New("node not found")
	}

	var inbounds []any
	for _, in := range st.Inbounds {
		if in.NodeID != nodeID || !in.Enabled {
			continue
		}

		var clients []any
		for _, binding := range st.InboundClients {
			if binding.InboundID != in.ID || !binding.Enabled {
				continue
			}
			u := findUser(&st, binding.UserID)
			if u == nil || u.Status != "active" {
				continue
			}
			cred, err := s.crypt.Open(binding.CredentialEnc)
			if err != nil {
				return nil, Node{}, fmt.Errorf("decrypt inbound credential: %w", err)
			}
			email := binding.Email
			if email == "" {
				email = "gb:" + u.ID
			}

			switch in.Protocol {
			case "vless":
				item := map[string]any{"id": cred, "email": email}
				if in.TLSMode == "reality" && in.Transport == "tcp" {
					item["flow"] = "xtls-rprx-vision"
				}
				clients = append(clients, item)
			case "vmess":
				clients = append(clients, map[string]any{"id": cred, "email": email})
			case "trojan":
				clients = append(clients, map[string]any{"password": cred, "email": email})
			case "shadowsocks":
				clients = append(clients, map[string]any{
					"password": cred,
					"method":   defaultString(in.ShadowsocksMethod, "aes-128-gcm"),
					"email":    email,
				})
			}
		}

		settings := map[string]any{}
		switch in.Protocol {
		case "vless":
			settings["users"] = clients
			settings["decryption"] = "none"
		case "vmess":
			settings["users"] = clients
		case "trojan":
			settings["users"] = clients
		case "shadowsocks":
			serverPassword, err := s.crypt.Open(in.ShadowsocksServerPasswordEnc)
			if err != nil {
				return nil, Node{}, fmt.Errorf("decrypt Shadowsocks server password: %w", err)
			}
			settings["method"] = defaultString(in.ShadowsocksMethod, "aes-128-gcm")
			settings["password"] = serverPassword
			settings["users"] = clients
			settings["network"] = "tcp,udp"
		default:
			continue
		}

		method := "raw"
		switch in.Transport {
		case "ws":
			method = "websocket"
		case "grpc":
			method = "grpc"
		}
		stream := map[string]any{"method": method}
		switch in.Transport {
		case "ws":
			ws := map[string]any{"path": defaultString(in.Path, "/")}
			if in.Host != "" {
				ws["host"] = in.Host
			}
			stream["wsSettings"] = ws
		case "grpc":
			stream["grpcSettings"] = map[string]any{"serviceName": defaultString(in.ServiceName, "gamebridge")}
		}

		switch in.TLSMode {
		case "tls":
			stream["security"] = "tls"
			stream["tlsSettings"] = map[string]any{
				"certificates": []any{map[string]any{
					"certificateFile": in.CertFile,
					"keyFile":         in.KeyFile,
				}},
			}
		case "reality":
			priv, err := s.crypt.Open(in.RealityPrivateKeyEnc)
			if err != nil {
				return nil, Node{}, fmt.Errorf("decrypt REALITY private key: %w", err)
			}
			stream["security"] = "reality"
			stream["realitySettings"] = map[string]any{
				"show":        false,
				"target":      in.RealityDest,
				"xver":        0,
				"serverNames": in.RealityServerNames,
				"privateKey":  priv,
				"shortIds":    in.RealityShortIDs,
			}
		default:
			stream["security"] = "none"
		}

		inbounds = append(inbounds, map[string]any{
			"tag":            "gb-in-" + in.ID,
			"listen":         defaultString(in.Listen, "0.0.0.0"),
			"port":           in.Port,
			"protocol":       in.Protocol,
			"settings":       settings,
			"streamSettings": stream,
			"sniffing": map[string]any{
				"enabled":      true,
				"destOverride": []string{"http", "tls", "quic"},
			},
		})
	}

	outbounds, err := s.buildXrayOutbounds(st, nodeID)
	if err != nil {
		return nil, Node{}, err
	}

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"api": map[string]any{
			"tag":      "api",
			"listen":   "127.0.0.1:10085",
			"services": []string{"StatsService"},
		},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
					"statsUserOnline":   true,
				},
			},
			"system": map[string]any{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"stats":     map[string]any{},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   buildXrayRouting(st, nodeID),
	}

	return cfg, *node, nil
}

func (s *Server) deployXrayNode(nodeID string) error {
	if err := s.ensureRealityKeys(nodeID); err != nil {
		s.setNodeInboundStatus(nodeID, "degraded")
		return err
	}
	if err := s.ensureShadowsocksServerPasswords(nodeID); err != nil {
		s.setNodeInboundStatus(nodeID, "degraded")
		return err
	}
	cfg, node, err := s.buildXrayConfig(nodeID)
	if err != nil {
		s.setNodeInboundStatus(nodeID, "degraded")
		return err
	}
	if err := s.agentJSON(node, http.MethodPost, "/v1/xray/apply", xrayApplyRequest{Config: cfg}, nil); err != nil {
		s.setNodeInboundStatus(nodeID, "degraded")
		return err
	}
	s.setNodeInboundStatus(nodeID, "online")
	return nil
}

func (s *Server) setNodeInboundStatus(nodeID, status string) {
	_ = s.store.Update(func(st *State) error {
		for i := range st.Inbounds {
			if st.Inbounds[i].NodeID == nodeID {
				st.Inbounds[i].Status = status
			}
		}
		return nil
	})
}

func (s *Server) xrayNodeStatus(nodeID string) (map[string]any, error) {
	var node Node
	if err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n
		return nil
	}); err != nil {
		return nil, err
	}
	var out map[string]any
	if err := s.agentJSON(node, http.MethodGet, "/v1/xray/status", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) installXrayOnNode(nodeID string) (map[string]any, error) {
	var node Node
	if err := s.store.Read(func(st State) error {
		n := findNode(&st, nodeID)
		if n == nil {
			return errors.New("node not found")
		}
		node = *n
		return nil
	}); err != nil {
		return nil, err
	}
	var out map[string]any
	if err := s.agentJSON(node, http.MethodPost, "/v1/xray/install", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) attachUserToInbound(inboundID, userID string) (InboundClient, error) {
	var protocol string
	var username string
	if err := s.store.Read(func(st State) error {
		in := findInbound(&st, inboundID)
		if in == nil {
			return errors.New("inbound not found")
		}
		u := findUser(&st, userID)
		if u == nil {
			return errors.New("user not found")
		}
		if u.Status != "active" {
			return errors.New("user is not active")
		}
		if findInboundClient(&st, inboundID, userID) != nil {
			return errors.New("user is already attached to inbound")
		}
		protocol = in.Protocol
		username = u.Username
		return nil
	}); err != nil {
		return InboundClient{}, err
	}

	cred := credentialForProtocol(protocol)
	enc, err := s.crypt.Seal(cred)
	if err != nil {
		return InboundClient{}, err
	}
	binding := InboundClient{
		ID:            randomHex(12),
		InboundID:     inboundID,
		UserID:        userID,
		Email:         "gb:" + username,
		CredentialEnc: enc,
		Enabled:       true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.store.Update(func(st *State) error {
		st.InboundClients = append(st.InboundClients, binding)
		return nil
	}); err != nil {
		return InboundClient{}, err
	}

	var nodeID string
	_ = s.store.Read(func(st State) error {
		if in := findInbound(&st, inboundID); in != nil {
			nodeID = in.NodeID
		}
		return nil
	})
	if nodeID != "" {
		if err := s.deployXrayNode(nodeID); err != nil {
			return binding, fmt.Errorf("user attached but Xray deploy failed: %w", err)
		}
	}
	return binding, nil
}

func (s *Server) detachUserFromInbound(inboundID, userID string) error {
	var nodeID string
	if err := s.store.Update(func(st *State) error {
		in := findInbound(st, inboundID)
		if in == nil {
			return errors.New("inbound not found")
		}
		nodeID = in.NodeID
		out := st.InboundClients[:0]
		for _, x := range st.InboundClients {
			if !(x.InboundID == inboundID && x.UserID == userID) {
				out = append(out, x)
			}
		}
		st.InboundClients = out
		return nil
	}); err != nil {
		return err
	}
	return s.deployXrayNode(nodeID)
}

func (s *Server) inboundDetail(id string) (map[string]any, error) {
	var result map[string]any
	if err := s.store.Read(func(st State) error {
		in := findInbound(&st, id)
		if in == nil {
			return errors.New("inbound not found")
		}
		var clients []map[string]any
		for _, c := range st.InboundClients {
			if c.InboundID != id {
				continue
			}
			username := c.UserID
			if u := findUser(&st, c.UserID); u != nil {
				username = u.Username
			}
			clients = append(clients, map[string]any{
				"id":         c.ID,
				"user_id":    c.UserID,
				"username":   username,
				"email":      c.Email,
				"enabled":    c.Enabled,
				"created_at": c.CreatedAt,
			})
		}
		result = map[string]any{"inbound": *in, "clients": clients}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) xrayShareLink(inboundID, userID string) (string, error) {
	var st State
	_ = s.store.Read(func(x State) error { st = x; return nil })
	in := findInbound(&st, inboundID)
	if in == nil {
		return "", errors.New("inbound not found")
	}
	node := findNode(&st, in.NodeID)
	if node == nil {
		return "", errors.New("node not found")
	}
	u := findUser(&st, userID)
	if u == nil {
		return "", errors.New("user not found")
	}
	b := findInboundClient(&st, inboundID, userID)
	if b == nil || !b.Enabled {
		return "", errors.New("user is not attached to inbound")
	}
	cred, err := s.crypt.Open(b.CredentialEnc)
	if err != nil {
		return "", err
	}

	host := node.PublicIP
	if in.ServerName != "" {
		host = in.ServerName
	}
	name := url.QueryEscape("GameBridge-" + in.Name + "-" + u.Username)

	q := url.Values{}
	q.Set("type", in.Transport)

	switch in.Transport {
	case "ws":
		q.Set("path", defaultString(in.Path, "/"))
		if in.Host != "" {
			q.Set("host", in.Host)
		}
	case "grpc":
		q.Set("serviceName", defaultString(in.ServiceName, "gamebridge"))
		q.Set("mode", "gun")
	}

	if in.TLSMode == "tls" {
		q.Set("security", "tls")
		if in.ServerName != "" {
			q.Set("sni", in.ServerName)
		}
	}
	if in.TLSMode == "reality" {
		q.Set("security", "reality")
		q.Set("pbk", in.RealityPublicKey)
		q.Set("fp", defaultString(in.RealityFingerprint, "chrome"))
		if len(in.RealityServerNames) > 0 {
			q.Set("sni", in.RealityServerNames[0])
		}
		if len(in.RealityShortIDs) > 0 {
			q.Set("sid", in.RealityShortIDs[0])
		}
		if in.Protocol == "vless" && in.Transport == "tcp" {
			q.Set("flow", "xtls-rprx-vision")
		}
	}

	switch in.Protocol {
	case "vless":
		q.Set("encryption", "none")
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", cred, host, in.Port, q.Encode(), name), nil
	case "trojan":
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", url.QueryEscape(cred), host, in.Port, q.Encode(), name), nil
	case "vmess":
		obj := map[string]string{
			"v":    "2",
			"ps":   "GameBridge-" + in.Name + "-" + u.Username,
			"add":  host,
			"port": strconv.Itoa(in.Port),
			"id":   cred,
			"aid":  "0",
			"net":  in.Transport,
			"type": "none",
			"host": in.Host,
			"path": in.Path,
			"tls":  map[bool]string{true: "tls", false: ""}[in.TLSMode == "tls"],
			"sni":  in.ServerName,
		}
		raw, _ := json.Marshal(obj)
		return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
	case "shadowsocks":
		method := defaultString(in.ShadowsocksMethod, "aes-128-gcm")
		auth := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + cred))
		return fmt.Sprintf("ss://%s@%s:%d#%s", auth, host, in.Port, name), nil
	default:
		return "", errors.New("share link is not supported for protocol")
	}
}

func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/sub/")
	var cfgs []string
	name := "GameBridge"

	var st State
	_ = s.store.Read(func(x State) error { st = x; return nil })

	var target *User
	for i := range st.Users {
		if st.Users[i].SubscriptionToken == token && st.Users[i].Status == "active" {
			target = &st.Users[i]
			break
		}
	}
	if target == nil {
		http.NotFound(w, r)
		return
	}

	for _, p := range st.VPNPeers {
		if p.UserID == target.ID && p.Enabled {
			if c, err := s.crypt.Open(p.ConfigEnc); err == nil {
				cfgs = append(cfgs, c)
			}
		}
	}

	for _, c := range st.InboundClients {
		if c.UserID != target.ID || !c.Enabled {
			continue
		}
		if link, err := s.xrayShareLink(c.InboundID, target.ID); err == nil {
			cfgs = append(cfgs, link)
		}
	}

	if st.Settings["site_name"] != "" {
		name = st.Settings["site_name"]
	}
	if len(cfgs) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# %s subscription\n\n%s\n", name, strings.Join(cfgs, "\n"))
}

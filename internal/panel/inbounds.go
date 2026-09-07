// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type inboundCreate struct {
	Name               string   `json:"name"`
	Protocol           string   `json:"protocol"`
	NodeID             string   `json:"node_id"`
	Listen             string   `json:"listen"`
	Port               int      `json:"port"`
	Transport          string   `json:"transport"`
	TLSMode            string   `json:"tls_mode"`
	Enabled            bool     `json:"enabled"`
	Remark             string   `json:"remark"`
	Path               string   `json:"path"`
	Host               string   `json:"host"`
	ServiceName        string   `json:"service_name"`
	ServerName         string   `json:"server_name"`
	CertFile           string   `json:"cert_file"`
	KeyFile            string   `json:"key_file"`
	RealityDest        string   `json:"reality_dest"`
	RealityServerNames []string `json:"reality_server_names"`
	RealityShortIDs    []string `json:"reality_short_ids"`
	RealityFingerprint string   `json:"reality_fingerprint"`
	ShadowsocksMethod  string   `json:"shadowsocks_method"`
}

func validInboundProtocol(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "vless", "vmess", "trojan", "shadowsocks":
		return true
	default:
		return false
	}
}

func validTransport(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "tcp", "ws", "grpc":
		return true
	default:
		return false
	}
}

func validTLSMode(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "tls", "reality":
		return true
	default:
		return false
	}
}

func normalizeInboundInput(in *inboundCreate) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	in.TLSMode = strings.ToLower(strings.TrimSpace(in.TLSMode))
	in.Listen = defaultString(strings.TrimSpace(in.Listen), "0.0.0.0")
	in.Transport = defaultString(in.Transport, "tcp")
	in.TLSMode = defaultString(in.TLSMode, "none")
	in.Path = defaultString(strings.TrimSpace(in.Path), "/")
	in.ServiceName = defaultString(strings.TrimSpace(in.ServiceName), "gamebridge")
	in.RealityFingerprint = defaultString(strings.TrimSpace(in.RealityFingerprint), "chrome")
	in.ShadowsocksMethod = defaultString(strings.TrimSpace(in.ShadowsocksMethod), "aes-128-gcm")

	if in.Name == "" {
		return errors.New("name is required")
	}
	if !validInboundProtocol(in.Protocol) {
		return errors.New("unsupported inbound protocol")
	}
	if !validTransport(in.Transport) {
		return errors.New("transport must be tcp, ws or grpc")
	}
	if !validTLSMode(in.TLSMode) {
		return errors.New("tls_mode must be none, tls or reality")
	}
	if in.Port < 1 || in.Port > 65535 {
		return errors.New("invalid port")
	}
	if in.TLSMode == "tls" && (in.CertFile == "" || in.KeyFile == "") {
		return errors.New("TLS requires cert_file and key_file on the target node")
	}
	if in.TLSMode == "reality" {
		if in.Protocol != "vless" && in.Protocol != "trojan" {
			return errors.New("REALITY is supported for VLESS/Trojan in this phase")
		}
		if in.Transport != "tcp" {
			return errors.New("REALITY is limited to TCP in this phase")
		}
		if in.RealityDest == "" || len(in.RealityServerNames) == 0 {
			return errors.New("REALITY requires reality_dest and at least one server name")
		}
		if len(in.RealityShortIDs) == 0 {
			in.RealityShortIDs = []string{randomHex(8)}
		}
	}
	return nil
}

func inboundFromCreate(in inboundCreate) Inbound {
	return Inbound{
		ID:                 randomHex(12),
		Name:               in.Name,
		Protocol:           in.Protocol,
		NodeID:             in.NodeID,
		Listen:             in.Listen,
		Port:               in.Port,
		Transport:          in.Transport,
		TLSMode:            in.TLSMode,
		Enabled:            true,
		Status:             "configured",
		Remark:             in.Remark,
		Path:               in.Path,
		Host:               in.Host,
		ServiceName:        in.ServiceName,
		ServerName:         in.ServerName,
		CertFile:           in.CertFile,
		KeyFile:            in.KeyFile,
		RealityDest:        in.RealityDest,
		RealityServerNames: in.RealityServerNames,
		RealityShortIDs:    in.RealityShortIDs,
		RealityFingerprint: in.RealityFingerprint,
		ShadowsocksMethod:  in.ShadowsocksMethod,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
}

func (s *Server) handleInbounds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var out []Inbound
		_ = s.store.Read(func(st State) error { out = st.Inbounds; return nil })
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in inboundCreate
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeInboundInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		obj := inboundFromCreate(in)
		if err := s.store.Update(func(st *State) error {
			if err := validateInboundTargetSpec(st, "", obj.Name, obj.NodeID, obj.Listen, obj.Port); err != nil {
				return err
			}
			st.Inbounds = append(st.Inbounds, obj)
			return nil
		}); err != nil {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}

		deployErr := s.deployXrayNode(obj.NodeID)
		s.audit(r, "create", "inbound:"+obj.Name)
		if deployErr != nil {
			jsonWrite(w, http.StatusCreated, map[string]any{"inbound": obj, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusCreated, map[string]any{"inbound": obj})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleInboundItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(rest), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, http.StatusNotFound, "inbound not found")
		return
	}
	id := parts[0]

	if len(parts) == 2 {
		switch parts[1] {
		case "enable", "disable", "redeploy", "summary":
			s.handleInboundControl(w, r, id, parts[1])
			return
		}
	}

	if len(parts) == 2 && parts[1] == "deploy" && r.Method == http.MethodPost {
		if requireRole(r, "operator") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var nodeID string
		_ = s.store.Read(func(st State) error {
			if in := findInbound(&st, id); in != nil {
				nodeID = in.NodeID
			}
			return nil
		})
		if nodeID == "" {
			jsonError(w, http.StatusNotFound, "inbound not found")
			return
		}
		if err := s.deployXrayNode(nodeID); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, "deploy", "inbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if len(parts) == 2 && parts[1] == "xray-status" && r.Method == http.MethodGet {
		var nodeID string
		_ = s.store.Read(func(st State) error {
			if in := findInbound(&st, id); in != nil {
				nodeID = in.NodeID
			}
			return nil
		})
		if nodeID == "" {
			jsonError(w, http.StatusNotFound, "inbound not found")
			return
		}
		out, err := s.xrayNodeStatus(nodeID)
		if err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		jsonWrite(w, http.StatusOK, out)
		return
	}

	if len(parts) == 2 && parts[1] == "xray-install" && r.Method == http.MethodPost {
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var nodeID string
		_ = s.store.Read(func(st State) error {
			if in := findInbound(&st, id); in != nil {
				nodeID = in.NodeID
			}
			return nil
		})
		if nodeID == "" {
			jsonError(w, http.StatusNotFound, "inbound not found")
			return
		}
		out, err := s.installXrayOnNode(nodeID)
		if err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, "install-xray", "node:"+nodeID)
		jsonWrite(w, http.StatusOK, out)
		return
	}

	if len(parts) == 2 && parts[1] == "users" {
		switch r.Method {
		case http.MethodGet:
			detail, err := s.inboundDetail(id)
			if err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonWrite(w, http.StatusOK, detail)
		case http.MethodPost:
			if requireRole(r, "admin") != nil {
				jsonError(w, http.StatusForbidden, "forbidden")
				return
			}
			var in struct {
				UserID string `json:"user_id"`
			}
			if err := decodeJSON(r, &in); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			binding, err := s.attachUserToInbound(id, in.UserID)
			if err != nil {
				if strings.Contains(err.Error(), "deploy failed") {
					jsonWrite(w, http.StatusCreated, map[string]any{"binding": binding, "deploy_error": err.Error()})
					return
				}
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			s.audit(r, "attach-user", "inbound:"+id+"/user:"+in.UserID)
			jsonWrite(w, http.StatusCreated, map[string]any{"binding": binding})
		default:
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) >= 3 && parts[1] == "users" {
		userID := parts[2]
		if len(parts) == 4 && parts[3] == "share" && r.Method == http.MethodGet {
			link, err := s.xrayShareLink(id, userID)
			if err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			jsonWrite(w, http.StatusOK, map[string]string{"link": link})
			return
		}
		if len(parts) == 3 && r.Method == http.MethodDelete {
			if requireRole(r, "admin") != nil {
				jsonError(w, http.StatusForbidden, "forbidden")
				return
			}
			if err := s.detachUserFromInbound(id, userID); err != nil {
				jsonError(w, http.StatusBadGateway, err.Error())
				return
			}
			s.audit(r, "detach-user", "inbound:"+id+"/user:"+userID)
			jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		detail, err := s.inboundDetail(id)
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonWrite(w, http.StatusOK, detail)

	case http.MethodPut:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in inboundCreate
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeInboundInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updateInboundTransactional(s.store, id, in, s.deployXrayNode); err != nil {
			if isInboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
				return
			}
			if err.Error() == "inbound not found" {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonError(w, http.StatusConflict, err.Error())
			return
		}
		s.audit(r, "update", "inbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var nodeID string
		err := s.store.Update(func(st *State) error {
			in := findInbound(st, id)
			if in == nil {
				return errors.New("inbound not found")
			}
			nodeID = in.NodeID
			st.Inbounds = deleteByID(st.Inbounds, id, func(x Inbound) string { return x.ID })
			out := st.InboundClients[:0]
			for _, c := range st.InboundClients {
				if c.InboundID != id {
					out = append(out, c)
				}
			}
			st.InboundClients = out
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		deployErr := s.deployXrayNode(nodeID)
		s.audit(r, "delete", "inbound:"+id)
		if deployErr != nil {
			jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

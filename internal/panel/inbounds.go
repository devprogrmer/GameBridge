// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type inboundCreate struct {
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	NodeID    string `json:"node_id"`
	Listen    string `json:"listen"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	TLSMode   string `json:"tls_mode"`
	Enabled   bool   `json:"enabled"`
	Remark    string `json:"remark"`
}

func validInboundProtocol(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "vless", "vmess", "trojan", "shadowsocks", "wireguard", "hysteria2":
		return true
	default:
		return false
	}
}

func (s *Server) handleInbounds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var out []Inbound
		_ = s.store.Read(func(st State) error {
			out = st.Inbounds
			return nil
		})
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

		in.Name = strings.TrimSpace(in.Name)
		in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
		in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
		in.TLSMode = strings.ToLower(strings.TrimSpace(in.TLSMode))

		if in.Name == "" {
			jsonError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !validInboundProtocol(in.Protocol) {
			jsonError(w, http.StatusBadRequest, "unsupported inbound protocol")
			return
		}
		if in.Port < 1 || in.Port > 65535 {
			jsonError(w, http.StatusBadRequest, "invalid port")
			return
		}

		if in.Listen == "" {
			in.Listen = "0.0.0.0"
		}
		if in.Transport == "" {
			in.Transport = "tcp"
		}
		if in.TLSMode == "" {
			in.TLSMode = "none"
		}

		obj := Inbound{
			ID:        randomHex(12),
			Name:      in.Name,
			Protocol:  in.Protocol,
			NodeID:    in.NodeID,
			Listen:    in.Listen,
			Port:      in.Port,
			Transport: in.Transport,
			TLSMode:   in.TLSMode,
			Enabled:   true,
			Remark:    in.Remark,
			Status:    "configured",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		err := s.store.Update(func(st *State) error {
			if findNode(st, obj.NodeID) == nil {
				return errors.New("node not found")
			}
			for _, x := range st.Inbounds {
				if x.NodeID == obj.NodeID && x.Port == obj.Port && x.Listen == obj.Listen {
					return errors.New("listen address/port is already used by another inbound")
				}
				if strings.EqualFold(x.Name, obj.Name) {
					return errors.New("inbound name already exists")
				}
			}
			st.Inbounds = append(st.Inbounds, obj)
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}

		s.audit(r, "create", "inbound:"+obj.Name)
		jsonWrite(w, http.StatusCreated, obj)

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleInboundItem(w http.ResponseWriter, r *http.Request, id string) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		jsonError(w, http.StatusNotFound, "inbound not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		var out *Inbound
		_ = s.store.Read(func(st State) error {
			if x := findInbound(&st, id); x != nil {
				cp := *x
				out = &cp
			}
			return nil
		})
		if out == nil {
			jsonError(w, http.StatusNotFound, "inbound not found")
			return
		}
		jsonWrite(w, http.StatusOK, out)

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
		if !validInboundProtocol(in.Protocol) || in.Port < 1 || in.Port > 65535 {
			jsonError(w, http.StatusBadRequest, "invalid inbound settings")
			return
		}

		err := s.store.Update(func(st *State) error {
			x := findInbound(st, id)
			if x == nil {
				return errors.New("inbound not found")
			}
			x.Name = strings.TrimSpace(in.Name)
			x.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
			x.NodeID = in.NodeID
			x.Listen = defaultString(in.Listen, "0.0.0.0")
			x.Port = in.Port
			x.Transport = defaultString(strings.ToLower(strings.TrimSpace(in.Transport)), "tcp")
			x.TLSMode = defaultString(strings.ToLower(strings.TrimSpace(in.TLSMode)), "none")
			x.Enabled = in.Enabled
			x.Remark = in.Remark
			x.UpdatedAt = time.Now().UTC()
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		s.audit(r, "update", "inbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		_ = s.store.Update(func(st *State) error {
			st.Inbounds = deleteByID(st.Inbounds, id, func(x Inbound) string { return x.ID })
			return nil
		})
		s.audit(r, "delete", "inbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

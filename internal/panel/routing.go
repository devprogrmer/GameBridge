// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var xrayTagPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)

type outboundInput struct {
	Name     string `json:"name"`
	NodeID   string `json:"node_id"`
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  bool   `json:"enabled"`
	Remark   string `json:"remark"`
}

func normalizeOutboundInput(in *outboundInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.Tag = strings.TrimSpace(in.Tag)
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Address = strings.TrimSpace(in.Address)
	in.Username = strings.TrimSpace(in.Username)

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
	case "socks", "http":
		if in.Address == "" || in.Port < 1 || in.Port > 65535 {
			return errors.New("proxy outbound requires address and valid port")
		}
	default:
		return errors.New("protocol must be freedom, blackhole, socks or http")
	}
	return nil
}

func (s *Server) handleOutbounds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var out []Outbound
		_ = s.store.Read(func(st State) error {
			out = st.Outbounds
			return nil
		})
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in outboundInput
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeOutboundInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		passwordEnc := ""
		if in.Password != "" {
			var err error
			passwordEnc, err = s.crypt.Seal(in.Password)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		obj := Outbound{
			ID:          randomHex(12),
			Name:        in.Name,
			NodeID:      in.NodeID,
			Tag:         in.Tag,
			Protocol:    in.Protocol,
			Address:     in.Address,
			Port:        in.Port,
			Username:    in.Username,
			PasswordEnc: passwordEnc,
			Enabled:     true,
			Remark:      in.Remark,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}
		if err := s.store.Update(func(st *State) error {
			if findNode(st, obj.NodeID) == nil {
				return errors.New("node not found")
			}
			for _, x := range st.Outbounds {
				if x.NodeID == obj.NodeID && strings.EqualFold(x.Tag, obj.Tag) {
					return errors.New("outbound tag already exists on node")
				}
				if x.NodeID == obj.NodeID && strings.EqualFold(x.Name, obj.Name) {
					return errors.New("outbound name already exists on node")
				}
			}
			st.Outbounds = append(st.Outbounds, obj)
			return nil
		}); err != nil {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}
		deployErr := s.deployXrayNode(obj.NodeID)
		s.audit(r, "create", "outbound:"+obj.Name)
		if deployErr != nil {
			jsonWrite(w, http.StatusCreated, map[string]any{"outbound": obj, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusCreated, map[string]any{"outbound": obj})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleOutboundItem(w http.ResponseWriter, r *http.Request, id string) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		jsonError(w, http.StatusNotFound, "outbound not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		var out *Outbound
		_ = s.store.Read(func(st State) error {
			if x := findOutbound(&st, id); x != nil {
				cp := *x
				out = &cp
			}
			return nil
		})
		if out == nil {
			jsonError(w, http.StatusNotFound, "outbound not found")
			return
		}
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPut:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in outboundInput
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeOutboundInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		var oldNodeID string
		var passwordEnc string
		err := s.store.Read(func(st State) error {
			x := findOutbound(&st, id)
			if x == nil {
				return errors.New("outbound not found")
			}
			oldNodeID = x.NodeID
			passwordEnc = x.PasswordEnc
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		if in.Password != "" {
			passwordEnc, err = s.crypt.Seal(in.Password)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		err = s.store.Update(func(st *State) error {
			if findNode(st, in.NodeID) == nil {
				return errors.New("node not found")
			}
			for _, other := range st.Outbounds {
				if other.ID != id && other.NodeID == in.NodeID && strings.EqualFold(other.Tag, in.Tag) {
					return errors.New("outbound tag already exists on node")
				}
			}
			x := findOutbound(st, id)
			if x == nil {
				return errors.New("outbound not found")
			}
			x.Name = in.Name
			x.NodeID = in.NodeID
			x.Tag = in.Tag
			x.Protocol = in.Protocol
			x.Address = in.Address
			x.Port = in.Port
			x.Username = in.Username
			x.PasswordEnc = passwordEnc
			x.Enabled = in.Enabled
			x.Remark = in.Remark
			x.UpdatedAt = time.Now().UTC()
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}
		if oldNodeID != "" && oldNodeID != in.NodeID {
			_ = s.deployXrayNode(oldNodeID)
		}
		if err := s.deployXrayNode(in.NodeID); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, "update", "outbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var nodeID string
		err := s.store.Update(func(st *State) error {
			x := findOutbound(st, id)
			if x == nil {
				return errors.New("outbound not found")
			}
			nodeID = x.NodeID
			for _, rr := range st.RoutingRules {
				if rr.OutboundID == id {
					return errors.New("outbound is used by a routing rule")
				}
			}
			st.Outbounds = deleteByID(st.Outbounds, id, func(x Outbound) string { return x.ID })
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusConflict, err.Error())
			return
		}
		deployErr := s.deployXrayNode(nodeID)
		s.audit(r, "delete", "outbound:"+id)
		if deployErr != nil {
			jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func normalizeRoutingRule(in *RoutingRule) error {
	in.Name = strings.TrimSpace(in.Name)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.Ports = strings.TrimSpace(in.Ports)
	in.Network = strings.ToLower(strings.TrimSpace(in.Network))
	in.OutboundID = strings.TrimSpace(in.OutboundID)
	if in.Name == "" || in.NodeID == "" || in.OutboundID == "" {
		return errors.New("name, node_id and outbound_id are required")
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
	if r.OutboundID != "direct" && r.OutboundID != "blocked" {
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
		switch x.Protocol {
		case "freedom", "blackhole":
		case "socks", "http":
			settings := map[string]any{"address": x.Address, "port": x.Port}
			if x.Username != "" {
				settings["user"] = x.Username
				if x.PasswordEnc != "" {
					pass, err := s.crypt.Open(x.PasswordEnc)
					if err != nil {
						return nil, err
					}
					settings["pass"] = pass
				}
			}
			item["settings"] = settings
		default:
			continue
		}
		out = append(out, item)
	}
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
		target := r.OutboundID
		if target != "direct" && target != "blocked" {
			if x := findOutbound(&st, target); x != nil && x.Enabled {
				target = x.Tag
			} else {
				continue
			}
		}
		rule := map[string]any{
			"type":        "field",
			"outboundTag": target,
			"ruleTag":     "gb-route-" + r.ID,
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
	return map[string]any{"domainStrategy": "AsIs", "rules": out}
}

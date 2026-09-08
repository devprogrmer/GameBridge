// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type outboundDeploymentError struct{ cause error }

func (e *outboundDeploymentError) Error() string { return e.cause.Error() }
func (e *outboundDeploymentError) Unwrap() error { return e.cause }

func isOutboundDeploymentError(err error) bool {
	var target *outboundDeploymentError
	return errors.As(err, &target)
}

func validateOutboundTargetSpec(st *State, outboundID, name, nodeID, tag string) error {
	node := findNode(st, nodeID)
	if node == nil {
		return errors.New("node not found")
	}
	if !node.Enabled {
		return errors.New("node is disabled")
	}
	if node.Maintenance {
		return errors.New("node is in maintenance")
	}

	name = strings.TrimSpace(name)
	tag = strings.TrimSpace(tag)
	for _, x := range st.Outbounds {
		if x.ID == outboundID {
			continue
		}
		if x.NodeID == nodeID && strings.EqualFold(strings.TrimSpace(x.Tag), tag) {
			return errors.New("outbound tag already exists on node")
		}
		if x.NodeID == nodeID && strings.EqualFold(strings.TrimSpace(x.Name), name) {
			return errors.New("outbound name already exists on node")
		}
	}
	return nil
}

func cloneOutboundValue(in Outbound) Outbound { return in }

func outboundDeployOrder(oldNodeID, newNodeID string) []string {
	if oldNodeID == "" {
		if newNodeID == "" {
			return nil
		}
		return []string{newNodeID}
	}
	if newNodeID == "" || oldNodeID == newNodeID {
		return []string{oldNodeID}
	}
	return []string{oldNodeID, newNodeID}
}

func redeployOutboundNodes(nodes []string, deploy func(string) error) error {
	var errs []string
	seen := map[string]bool{}
	for _, nodeID := range nodes {
		if nodeID == "" || seen[nodeID] {
			continue
		}
		seen[nodeID] = true
		if err := deploy(nodeID); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", nodeID, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func createOutboundTransactional(store *Store, obj Outbound, deploy func(string) error) error {
	if err := store.Update(func(st *State) error {
		if err := validateOutboundTargetSpec(st, "", obj.Name, obj.NodeID, obj.Tag); err != nil {
			return err
		}
		st.Outbounds = append(st.Outbounds, obj)
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(obj.NodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", obj.NodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			st.Outbounds = deleteByID(st.Outbounds, obj.ID, func(x Outbound) string { return x.ID })
			return nil
		})
		recoveryErr := deploy(obj.NodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func updateOutboundTransactional(store *Store, id string, in outboundInput, passwordEnc string, deploy func(string) error) error {
	var before Outbound
	var oldNodeID, newNodeID string

	if err := store.Update(func(st *State) error {
		x := findOutbound(st, id)
		if x == nil {
			return errors.New("outbound not found")
		}
		if err := validateOutboundTargetSpec(st, id, in.Name, in.NodeID, in.Tag); err != nil {
			return err
		}
		before = cloneOutboundValue(*x)
		oldNodeID = x.NodeID
		newNodeID = in.NodeID

		x.Name = in.Name
		x.NodeID = in.NodeID
		x.Tag = in.Tag
		x.Protocol = in.Protocol
		x.Address = in.Address
		x.Port = in.Port
		x.Username = in.Username
		x.PasswordEnc = passwordEnc
		x.Transport = in.Transport
		x.TLSMode = in.TLSMode
		x.Path = in.Path
		x.Host = in.Host
		x.ServiceName = in.ServiceName
		x.ServerName = in.ServerName
		x.AllowInsecure = in.AllowInsecure
		x.Fingerprint = in.Fingerprint
		x.Flow = in.Flow
		x.ShadowsocksMethod = in.ShadowsocksMethod
		x.RealityPublicKey = in.RealityPublicKey
		x.RealityShortID = in.RealityShortID
		x.WireGuardInterface = in.WireGuardInterface
		x.WireGuardAddress = in.WireGuardAddress
		x.WireGuardPeerPublicKey = in.WireGuardPeerPublicKey
		x.WireGuardAllowedIPs = append([]string(nil), in.WireGuardAllowedIPs...)
		x.WireGuardKeepalive = in.WireGuardKeepalive
		x.WireGuardMTU = in.WireGuardMTU
		x.TorSOCKSPort = in.TorSOCKSPort
		x.OpenVPNInterface = in.OpenVPNInterface
		x.OpenVPNRoutingTable = in.OpenVPNRoutingTable
		x.OpenVPNMark = in.OpenVPNMark
		x.CustomXrayProtocol = in.CustomXrayProtocol
		x.Enabled = in.Enabled
		x.Remark = in.Remark
		x.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}

	nodes := outboundDeployOrder(oldNodeID, newNodeID)
	for _, nodeID := range nodes {
		if err := deploy(nodeID); err != nil {
			deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
			rollbackErr := store.Update(func(st *State) error {
				x := findOutbound(st, id)
				if x == nil {
					return errors.New("outbound disappeared during rollback")
				}
				*x = before
				return nil
			})
			recoveryErr := redeployOutboundNodes(nodes, deploy)
			switch {
			case rollbackErr != nil && recoveryErr != nil:
				return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
			case rollbackErr != nil:
				return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
			case recoveryErr != nil:
				return &outboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
			default:
				return &outboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
			}
		}
	}
	return nil
}

func setOutboundEnabledTransactional(store *Store, id string, enabled bool, deploy func(string) error) error {
	var before bool
	var nodeID string

	if err := store.Update(func(st *State) error {
		x := findOutbound(st, id)
		if x == nil {
			return errors.New("outbound not found")
		}
		if enabled {
			if err := validateOutboundTargetSpec(st, id, x.Name, x.NodeID, x.Tag); err != nil {
				return err
			}
		} else {
			for _, rr := range st.RoutingRules {
				if rr.Enabled && rr.OutboundID == id {
					return errors.New("outbound is used by an enabled routing rule")
				}
			}
		}
		before = x.Enabled
		nodeID = x.NodeID
		x.Enabled = enabled
		x.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			x := findOutbound(st, id)
			if x == nil {
				return errors.New("outbound disappeared during rollback")
			}
			x.Enabled = before
			x.UpdatedAt = time.Now().UTC()
			return nil
		})
		recoveryErr := deploy(nodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func deleteOutboundTransactional(store *Store, id string, deploy func(string) error) error {
	var before Outbound
	var index int
	var nodeID string

	if err := store.Update(func(st *State) error {
		x := findOutbound(st, id)
		if x == nil {
			return errors.New("outbound not found")
		}
		for _, rr := range st.RoutingRules {
			if rr.OutboundID == id {
				return errors.New("outbound is used by a routing rule")
			}
		}
		before = *x
		nodeID = x.NodeID
		index = -1
		for i := range st.Outbounds {
			if st.Outbounds[i].ID == id {
				index = i
				break
			}
		}
		st.Outbounds = deleteByID(st.Outbounds, id, func(x Outbound) string { return x.ID })
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			if findOutbound(st, id) != nil {
				return nil
			}
			if index < 0 || index > len(st.Outbounds) {
				st.Outbounds = append(st.Outbounds, before)
				return nil
			}
			st.Outbounds = append(st.Outbounds, Outbound{})
			copy(st.Outbounds[index+1:], st.Outbounds[index:])
			st.Outbounds[index] = before
			return nil
		})
		recoveryErr := deploy(nodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &outboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func (s *Server) outboundSummary(id string) (map[string]any, error) {
	var result map[string]any
	err := s.store.Read(func(st State) error {
		x := findOutbound(&st, id)
		if x == nil {
			return errors.New("outbound not found")
		}
		node := findNode(&st, x.NodeID)
		routingTotal := 0
		routingEnabled := 0
		for _, rr := range st.RoutingRules {
			if rr.OutboundID == id {
				routingTotal++
				if rr.Enabled {
					routingEnabled++
				}
			}
		}
		nodeStatus := "unknown"
		nodeEnabled := false
		nodeMaintenance := false
		if node != nil {
			nodeStatus = node.Status
			nodeEnabled = node.Enabled
			nodeMaintenance = node.Maintenance
		}
		result = map[string]any{
			"outbound":         *x,
			"routing_rules":    routingTotal,
			"enabled_rules":    routingEnabled,
			"node_status":      nodeStatus,
			"node_enabled":     nodeEnabled,
			"node_maintenance": nodeMaintenance,
			"ready":            x.Enabled && node != nil && node.Enabled && !node.Maintenance,
		}
		return nil
	})
	return result, err
}

func (s *Server) handleOutboundsV2(w http.ResponseWriter, r *http.Request) {
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

		secret := outboundInputSecret(in)
		if in.Protocol == "openvpn" {
			if strings.TrimSpace(in.Secret) == "" {
				jsonError(w, http.StatusBadRequest, "openvpn secret must contain the .ovpn profile")
				return
			}
			packed, err := encodeOpenVPNCredential(in.Secret, in.Password)
			if err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			secret = packed
		}
		if in.Protocol == "custom" && strings.TrimSpace(in.Secret) == "" {
			jsonError(w, http.StatusBadRequest, "custom xray secret must contain the outbound JSON config")
			return
		}
		if outboundProtocolNeedsSecret(in.Protocol) && secret == "" {
			jsonError(w, http.StatusBadRequest, "credential is required for selected outbound protocol")
			return
		}
		passwordEnc := ""
		if secret != "" {
			var err error
			passwordEnc, err = s.crypt.Seal(secret)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		obj := Outbound{
			ID:                     randomHex(12),
			Name:                   in.Name,
			NodeID:                 in.NodeID,
			Tag:                    in.Tag,
			Protocol:               in.Protocol,
			Address:                in.Address,
			Port:                   in.Port,
			Username:               in.Username,
			PasswordEnc:            passwordEnc,
			Transport:              in.Transport,
			TLSMode:                in.TLSMode,
			Path:                   in.Path,
			Host:                   in.Host,
			ServiceName:            in.ServiceName,
			ServerName:             in.ServerName,
			AllowInsecure:          in.AllowInsecure,
			Fingerprint:            in.Fingerprint,
			Flow:                   in.Flow,
			ShadowsocksMethod:      in.ShadowsocksMethod,
			RealityPublicKey:       in.RealityPublicKey,
			RealityShortID:         in.RealityShortID,
			WireGuardInterface:     in.WireGuardInterface,
			WireGuardAddress:       in.WireGuardAddress,
			WireGuardPeerPublicKey: in.WireGuardPeerPublicKey,
			WireGuardAllowedIPs:    append([]string(nil), in.WireGuardAllowedIPs...),
			WireGuardKeepalive:     in.WireGuardKeepalive,
			WireGuardMTU:           in.WireGuardMTU,
			TorSOCKSPort:           in.TorSOCKSPort,
			OpenVPNInterface:       in.OpenVPNInterface,
			OpenVPNRoutingTable:    in.OpenVPNRoutingTable,
			OpenVPNMark:            in.OpenVPNMark,
			CustomXrayProtocol:     in.CustomXrayProtocol,
			Enabled:                true,
			Remark:                 in.Remark,
			CreatedAt:              time.Now().UTC(),
			UpdatedAt:              time.Now().UTC(),
		}
		if err := createOutboundTransactional(s.store, obj, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "create", "outbound:"+obj.Name)
		jsonWrite(w, http.StatusCreated, map[string]any{"outbound": obj})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleOutboundItemV2(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(rest), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, http.StatusNotFound, "outbound not found")
		return
	}
	id := parts[0]

	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "summary":
			if r.Method != http.MethodGet {
				jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			out, err := s.outboundSummary(id)
			if err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonWrite(w, http.StatusOK, out)
			return

		case "enable", "disable":
			if r.Method != http.MethodPost {
				jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if requireRole(r, "operator") != nil {
				jsonError(w, http.StatusForbidden, "forbidden")
				return
			}
			err := setOutboundEnabledTransactional(s.store, id, action == "enable", s.deployOutboundNode)
			if err != nil {
				if isOutboundDeploymentError(err) {
					jsonError(w, http.StatusBadGateway, err.Error())
				} else if err.Error() == "outbound not found" {
					jsonError(w, http.StatusNotFound, err.Error())
				} else {
					jsonError(w, http.StatusConflict, err.Error())
				}
				return
			}
			s.audit(r, action, "outbound:"+id)
			jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
			return

		case "redeploy":
			if r.Method != http.MethodPost {
				jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			if requireRole(r, "operator") != nil {
				jsonError(w, http.StatusForbidden, "forbidden")
				return
			}
			var nodeID string
			if err := s.store.Read(func(st State) error {
				x := findOutbound(&st, id)
				if x == nil {
					return errors.New("outbound not found")
				}
				nodeID = x.NodeID
				return nil
			}); err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			if err := s.deployOutboundNode(nodeID); err != nil {
				jsonError(w, http.StatusBadGateway, err.Error())
				return
			}
			s.audit(r, "redeploy", "outbound:"+id)
			jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
			return
		default:
			jsonError(w, http.StatusNotFound, "outbound action not found")
			return
		}
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

		var passwordEnc string
		var oldProtocol string
		var oldCustomXrayProtocol string
		err := s.store.Read(func(st State) error {
			x := findOutbound(&st, id)
			if x == nil {
				return errors.New("outbound not found")
			}
			passwordEnc = x.PasswordEnc
			oldProtocol = x.Protocol
			oldCustomXrayProtocol = x.CustomXrayProtocol
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		secret := outboundInputSecret(in)
		if in.Protocol == "openvpn" {
			profile := strings.TrimSpace(in.Secret)
			authPassword := in.Password

			if oldProtocol == "openvpn" && passwordEnc != "" {
				plain, openErr := s.crypt.Open(passwordEnc)
				if openErr != nil {
					jsonError(w, http.StatusInternalServerError, openErr.Error())
					return
				}
				existing, decodeErr := decodeOpenVPNCredential(plain)
				if decodeErr != nil {
					jsonError(w, http.StatusInternalServerError, decodeErr.Error())
					return
				}
				if profile == "" {
					profile = existing.Profile
				}
				if authPassword == "" {
					authPassword = existing.Password
				}
			}

			if profile == "" {
				jsonError(w, http.StatusBadRequest, "openvpn secret must contain the .ovpn profile")
				return
			}
			packed, packErr := encodeOpenVPNCredential(profile, authPassword)
			if packErr != nil {
				jsonError(w, http.StatusBadRequest, packErr.Error())
				return
			}
			passwordEnc, err = s.crypt.Seal(packed)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else if in.Protocol == "custom" {
			if secret == "" {
				if oldProtocol != "custom" || passwordEnc == "" {
					jsonError(w, http.StatusBadRequest, "custom xray secret must contain the outbound JSON config")
					return
				}
				in.CustomXrayProtocol = oldCustomXrayProtocol
			} else {
				passwordEnc, err = s.crypt.Seal(secret)
				if err != nil {
					jsonError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		} else {
			if oldProtocol != in.Protocol && secret == "" {
				passwordEnc = ""
			}
			if secret != "" {
				passwordEnc, err = s.crypt.Seal(secret)
				if err != nil {
					jsonError(w, http.StatusInternalServerError, err.Error())
					return
				}
			}
		}
		if outboundProtocolNeedsSecret(in.Protocol) && passwordEnc == "" {
			jsonError(w, http.StatusBadRequest, "credential is required for selected outbound protocol")
			return
		}

		if err := updateOutboundTransactional(s.store, id, in, passwordEnc, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else if err.Error() == "outbound not found" {
				jsonError(w, http.StatusNotFound, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "update", "outbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		if err := deleteOutboundTransactional(s.store, id, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else if err.Error() == "outbound not found" {
				jsonError(w, http.StatusNotFound, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "delete", "outbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

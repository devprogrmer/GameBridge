// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net/http"
	"time"
)

type InboundSummary struct {
	Inbound         Inbound `json:"inbound"`
	AttachedUsers   int     `json:"attached_users"`
	EnabledUsers    int     `json:"enabled_users"`
	ActiveUsers     int     `json:"active_users"`
	NodeStatus      string  `json:"node_status"`
	NodeEnabled     bool    `json:"node_enabled"`
	NodeMaintenance bool    `json:"node_maintenance"`
}

func inboundSummary(st *State, id string, now time.Time) (InboundSummary, error) {
	in := findInbound(st, id)
	if in == nil {
		return InboundSummary{}, errors.New("inbound not found")
	}
	out := InboundSummary{Inbound: *in}
	if n := findNode(st, in.NodeID); n != nil {
		out.NodeStatus = n.Status
		out.NodeEnabled = n.Enabled
		out.NodeMaintenance = n.Maintenance
	}
	for _, b := range st.InboundClients {
		if b.InboundID != id {
			continue
		}
		out.AttachedUsers++
		if b.Enabled {
			out.EnabledUsers++
		}
		if !b.Enabled {
			continue
		}
		if u := findUser(st, b.UserID); u != nil {
			status, _ := userLifecycleStatus(st, *u, now)
			if status == UserStatusActive {
				out.ActiveUsers++
			}
		}
	}
	return out, nil
}

func validateInboundTarget(st *State, inboundID, nodeID, listen string, port int) error {
	n := findNode(st, nodeID)
	if n == nil {
		return errors.New("node not found")
	}
	if !n.Enabled {
		return errors.New("node is disabled")
	}
	if n.Maintenance {
		return errors.New("node is in maintenance")
	}
	for _, x := range st.Inbounds {
		if x.ID == inboundID {
			continue
		}
		if x.NodeID == nodeID && x.Port == port && x.Listen == listen {
			return errors.New("listen address/port is already used by another inbound")
		}
	}
	return nil
}

func (s *Server) handleInboundControl(w http.ResponseWriter, r *http.Request, id, action string) {
	switch action {
	case "summary":
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var out InboundSummary
		err := s.store.Read(func(st State) error {
			var e error
			out, e = inboundSummary(&st, id, time.Now().UTC())
			return e
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonWrite(w, http.StatusOK, out)
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
		err := s.store.Read(func(st State) error {
			in := findInbound(&st, id)
			if in == nil {
				return errors.New("inbound not found")
			}
			nodeID = in.NodeID
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		if err := s.deployXrayNode(nodeID); err != nil {
			jsonError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.audit(r, "redeploy", "inbound:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
		return

	case "enable", "disable":
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		enable := action == "enable"
		var nodeID string
		err := s.store.Update(func(st *State) error {
			in := findInbound(st, id)
			if in == nil {
				return errors.New("inbound not found")
			}
			if enable {
				if err := validateInboundTarget(st, in.ID, in.NodeID, in.Listen, in.Port); err != nil {
					return err
				}
			}
			in.Enabled = enable
			in.Status = map[bool]string{true: "configured", false: "disabled"}[enable]
			in.UpdatedAt = time.Now().UTC()
			nodeID = in.NodeID
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		deployErr := s.deployXrayNode(nodeID)
		s.audit(r, action, "inbound:"+id)
		if deployErr != nil {
			jsonWrite(w, http.StatusOK, map[string]any{"ok": true, "deploy_error": deployErr.Error()})
			return
		}
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

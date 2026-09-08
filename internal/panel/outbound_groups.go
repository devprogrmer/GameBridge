// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	outboundGroupStrategyRandom     = "random"
	outboundGroupStrategyRoundRobin = "round_robin"
	outboundGroupStrategyLeastPing  = "least_ping"
	outboundGroupStrategyLeastLoad  = "least_load"
)

func findOutboundGroup(st *State, id string) *OutboundGroup {
	for i := range st.OutboundGroups {
		if st.OutboundGroups[i].ID == id {
			return &st.OutboundGroups[i]
		}
	}
	return nil
}

func deleteOutboundGroupByID(groups []OutboundGroup, id string) []OutboundGroup {
	out := groups[:0]
	for _, group := range groups {
		if group.ID != id {
			out = append(out, group)
		}
	}
	return out
}

func cloneOutboundGroup(in OutboundGroup) OutboundGroup {
	out := in
	out.Members = append([]OutboundGroupMember(nil), in.Members...)
	return out
}

func normalizeOutboundGroupInput(group *OutboundGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	group.NodeID = strings.TrimSpace(group.NodeID)
	group.Strategy = strings.ToLower(strings.TrimSpace(group.Strategy))
	group.FallbackOutboundID = strings.TrimSpace(group.FallbackOutboundID)

	if group.Name == "" || group.NodeID == "" {
		return errors.New("name and node_id are required")
	}
	if group.Strategy == "" {
		group.Strategy = outboundGroupStrategyLeastPing
	}
	switch group.Strategy {
	case outboundGroupStrategyRandom,
		outboundGroupStrategyRoundRobin,
		outboundGroupStrategyLeastPing,
		outboundGroupStrategyLeastLoad:
	default:
		return errors.New("strategy must be random, round_robin, least_ping or least_load")
	}
	if len(group.Members) == 0 {
		return errors.New("outbound group requires at least one member")
	}
	if len(group.Members) > 32 {
		return errors.New("outbound group supports at most 32 members")
	}
	if group.FallbackOutboundID == "" {
		group.FallbackOutboundID = "blocked"
	}
	if group.Expected == 0 {
		group.Expected = 1
	}
	if group.Expected < 1 || group.Expected > len(group.Members) {
		return errors.New("expected must be between 1 and member count")
	}

	seen := map[string]bool{}
	for i := range group.Members {
		member := &group.Members[i]
		member.OutboundID = strings.TrimSpace(member.OutboundID)
		if member.OutboundID == "" {
			return errors.New("outbound group member outbound_id is required")
		}
		if seen[member.OutboundID] {
			return errors.New("outbound group has duplicate members")
		}
		seen[member.OutboundID] = true
		if member.Priority < 0 || member.Priority > 1000 {
			return errors.New("outbound group member priority must be between 0 and 1000")
		}
		if member.Weight == 0 {
			member.Weight = 1
		}
		if member.Weight < 1 || member.Weight > 100 {
			return errors.New("outbound group member weight must be between 1 and 100")
		}
	}

	sort.SliceStable(group.Members, func(i, j int) bool {
		if group.Members[i].Priority == group.Members[j].Priority {
			return group.Members[i].OutboundID < group.Members[j].OutboundID
		}
		return group.Members[i].Priority < group.Members[j].Priority
	})
	return nil
}

func validateOutboundGroupSpec(st *State, group OutboundGroup) error {
	node := findNode(st, group.NodeID)
	if node == nil {
		return errors.New("node not found")
	}
	if !node.Enabled {
		return errors.New("node is disabled")
	}
	if node.Maintenance {
		return errors.New("node is in maintenance")
	}

	for _, other := range st.OutboundGroups {
		if other.ID == group.ID {
			continue
		}
		if other.NodeID == group.NodeID && strings.EqualFold(strings.TrimSpace(other.Name), strings.TrimSpace(group.Name)) {
			return errors.New("outbound group name already exists on node")
		}
	}

	memberIDs := map[string]bool{}
	for _, member := range group.Members {
		out := findOutbound(st, member.OutboundID)
		if out == nil {
			return fmt.Errorf("outbound group member %s not found", member.OutboundID)
		}
		if out.NodeID != group.NodeID {
			return fmt.Errorf("outbound group member %s belongs to another node", member.OutboundID)
		}
		if !out.Enabled {
			return fmt.Errorf("outbound group member %s is disabled", member.OutboundID)
		}
		if out.Protocol == "blackhole" {
			return fmt.Errorf("blackhole outbound %s cannot be a load-balancer member", member.OutboundID)
		}
		memberIDs[member.OutboundID] = true
	}

	switch group.FallbackOutboundID {
	case "blocked", "direct":
	default:
		if memberIDs[group.FallbackOutboundID] {
			return errors.New("fallback outbound must not also be a group member")
		}
		out := findOutbound(st, group.FallbackOutboundID)
		if out == nil {
			return errors.New("fallback outbound not found")
		}
		if out.NodeID != group.NodeID {
			return errors.New("fallback outbound belongs to another node")
		}
		if !out.Enabled {
			return errors.New("fallback outbound is disabled")
		}
	}
	return nil
}

func validateOutboundGroupReferencesForChange(st *State, outboundID, newNodeID string, enabled bool) error {
	for _, group := range st.OutboundGroups {
		referenced := group.FallbackOutboundID == outboundID
		for _, member := range group.Members {
			if member.OutboundID == outboundID {
				referenced = true
				break
			}
		}
		if !referenced {
			continue
		}
		if !enabled {
			return fmt.Errorf("outbound is referenced by outbound group %q", group.Name)
		}
		if group.NodeID != newNodeID {
			return fmt.Errorf("outbound is referenced by outbound group %q on node %s", group.Name, group.NodeID)
		}
	}
	return nil
}

func outboundGroupReferencedByRouting(st *State, groupID string, enabledOnly bool) bool {
	for _, rule := range st.RoutingRules {
		if rule.OutboundGroupID != groupID {
			continue
		}
		if !enabledOnly || rule.Enabled {
			return true
		}
	}
	return false
}

func createOutboundGroupTransactional(store *Store, group OutboundGroup, deploy func(string) error) error {
	if err := store.Update(func(st *State) error {
		if err := validateOutboundGroupSpec(st, group); err != nil {
			return err
		}
		st.OutboundGroups = append(st.OutboundGroups, group)
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(group.NodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", group.NodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			st.OutboundGroups = deleteOutboundGroupByID(st.OutboundGroups, group.ID)
			return nil
		})
		recoveryErr := deploy(group.NodeID)
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

func updateOutboundGroupTransactional(store *Store, id string, in OutboundGroup, deploy func(string) error) error {
	var before OutboundGroup
	var oldNodeID, newNodeID string

	if err := store.Update(func(st *State) error {
		current := findOutboundGroup(st, id)
		if current == nil {
			return errors.New("outbound group not found")
		}
		if current.NodeID != in.NodeID && outboundGroupReferencedByRouting(st, id, false) {
			return errors.New("cannot move an outbound group that is referenced by routing rules")
		}
		if !in.Enabled && outboundGroupReferencedByRouting(st, id, true) {
			return errors.New("cannot disable an outbound group used by an enabled routing rule")
		}

		in.ID = current.ID
		in.CreatedAt = current.CreatedAt
		in.UpdatedAt = time.Now().UTC()
		if err := validateOutboundGroupSpec(st, in); err != nil {
			return err
		}

		before = cloneOutboundGroup(*current)
		oldNodeID = current.NodeID
		newNodeID = in.NodeID
		*current = cloneOutboundGroup(in)
		return nil
	}); err != nil {
		return err
	}

	nodes := outboundDeployOrder(oldNodeID, newNodeID)
	for _, nodeID := range nodes {
		if err := deploy(nodeID); err != nil {
			deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
			rollbackErr := store.Update(func(st *State) error {
				current := findOutboundGroup(st, id)
				if current == nil {
					return errors.New("outbound group disappeared during rollback")
				}
				*current = cloneOutboundGroup(before)
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

func deleteOutboundGroupTransactional(store *Store, id string, deploy func(string) error) error {
	var before OutboundGroup
	var index int
	var nodeID string

	if err := store.Update(func(st *State) error {
		group := findOutboundGroup(st, id)
		if group == nil {
			return errors.New("outbound group not found")
		}
		if outboundGroupReferencedByRouting(st, id, false) {
			return errors.New("outbound group is used by a routing rule")
		}
		before = cloneOutboundGroup(*group)
		nodeID = group.NodeID
		index = -1
		for i := range st.OutboundGroups {
			if st.OutboundGroups[i].ID == id {
				index = i
				break
			}
		}
		st.OutboundGroups = deleteOutboundGroupByID(st.OutboundGroups, id)
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			if findOutboundGroup(st, id) != nil {
				return nil
			}
			if index < 0 || index > len(st.OutboundGroups) {
				st.OutboundGroups = append(st.OutboundGroups, before)
				return nil
			}
			st.OutboundGroups = append(st.OutboundGroups, OutboundGroup{})
			copy(st.OutboundGroups[index+1:], st.OutboundGroups[index:])
			st.OutboundGroups[index] = before
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

func outboundGroupBalancerTag(groupID string) string {
	return "gb-bal-" + groupID
}

func outboundGroupMemberPrefix(groupID string) string {
	return "gbm-" + groupID + "-"
}

func outboundGroupAliasTag(groupID, outboundID string) string {
	return outboundGroupMemberPrefix(groupID) + outboundID
}

func cloneXrayOutboundMap(in map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func buildXrayBalancerAliases(st State, nodeID string, base []any) ([]any, error) {
	byTag := map[string]map[string]any{}
	for _, raw := range base {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		tag, _ := item["tag"].(string)
		if tag != "" {
			byTag[tag] = item
		}
	}

	var aliases []any
	for _, group := range st.OutboundGroups {
		if group.NodeID != nodeID || !group.Enabled {
			continue
		}
		for _, member := range group.Members {
			out := findOutbound(&st, member.OutboundID)
			if out == nil || !out.Enabled || out.NodeID != nodeID {
				continue
			}
			baseItem := byTag[out.Tag]
			if baseItem == nil {
				continue
			}
			alias, err := cloneXrayOutboundMap(baseItem)
			if err != nil {
				return nil, err
			}
			alias["tag"] = outboundGroupAliasTag(group.ID, out.ID)
			aliases = append(aliases, alias)
		}
	}
	return aliases, nil
}

func xrayOutboundGroupFallbackTag(st *State, group OutboundGroup) string {
	switch group.FallbackOutboundID {
	case "", "blocked":
		return "blocked"
	case "direct":
		return "direct"
	default:
		if out := findOutbound(st, group.FallbackOutboundID); out != nil && out.Enabled && out.NodeID == group.NodeID {
			return out.Tag
		}
		return "blocked"
	}
}

func xrayOutboundGroupStrategy(group OutboundGroup) map[string]any {
	strategyType := "leastPing"
	switch group.Strategy {
	case outboundGroupStrategyRandom:
		strategyType = "random"
	case outboundGroupStrategyRoundRobin:
		strategyType = "roundRobin"
	case outboundGroupStrategyLeastLoad:
		strategyType = "leastLoad"
	case outboundGroupStrategyLeastPing:
		strategyType = "leastPing"
	}

	strategy := map[string]any{
		"type":     strategyType,
		"settings": map[string]any{},
	}
	if group.Strategy != outboundGroupStrategyLeastLoad {
		return strategy
	}

	costs := make([]any, 0, len(group.Members))
	for _, member := range group.Members {
		weight := member.Weight
		if weight <= 0 {
			weight = 1
		}
		cost := float64((member.Priority+1)*100) / float64(weight)
		costs = append(costs, map[string]any{
			"regexp": false,
			"match":  outboundGroupAliasTag(group.ID, member.OutboundID),
			"value":  cost,
		})
	}
	expected := group.Expected
	if expected <= 0 {
		expected = 1
	}
	strategy["settings"] = map[string]any{
		"expected": expected,
		"costs":    costs,
	}
	return strategy
}

func buildXrayBalancers(st State, nodeID string) []any {
	var out []any
	for _, group := range st.OutboundGroups {
		if group.NodeID != nodeID || !group.Enabled {
			continue
		}
		if err := validateOutboundGroupSpec(&st, group); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"tag":         outboundGroupBalancerTag(group.ID),
			"selector":    []string{outboundGroupMemberPrefix(group.ID)},
			"fallbackTag": xrayOutboundGroupFallbackTag(&st, group),
			"strategy":    xrayOutboundGroupStrategy(group),
		})
	}
	return out
}

func outboundGroupSummary(st State, group OutboundGroup) map[string]any {
	members := make([]map[string]any, 0, len(group.Members))
	for _, member := range group.Members {
		entry := map[string]any{
			"outbound_id": member.OutboundID,
			"priority":    member.Priority,
			"weight":      member.Weight,
		}
		if out := findOutbound(&st, member.OutboundID); out != nil {
			entry["tag"] = out.Tag
			entry["name"] = out.Name
			entry["protocol"] = out.Protocol
			entry["health_status"] = normalizeOutboundHealthStatus(out.HealthStatus)
			entry["latency_ms"] = out.HealthLatencyMS
		}
		members = append(members, entry)
	}
	return map[string]any{
		"group":        group,
		"balancer_tag": outboundGroupBalancerTag(group.ID),
		"fallback_tag": xrayOutboundGroupFallbackTag(&st, group),
		"members":      members,
	}
}

func (s *Server) handleOutboundGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var groups []OutboundGroup
		_ = s.store.Read(func(st State) error {
			groups = append([]OutboundGroup(nil), st.OutboundGroups...)
			sort.Slice(groups, func(i, j int) bool {
				if groups[i].NodeID == groups[j].NodeID {
					return groups[i].Name < groups[j].Name
				}
				return groups[i].NodeID < groups[j].NodeID
			})
			return nil
		})
		jsonWrite(w, http.StatusOK, groups)

	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in OutboundGroup
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeOutboundGroupInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		now := time.Now().UTC()
		in.ID = randomHex(12)
		in.Enabled = true
		in.CreatedAt = now
		in.UpdatedAt = now

		if err := createOutboundGroupTransactional(s.store, in, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "create", "outbound-group:"+in.Name)
		jsonWrite(w, http.StatusCreated, map[string]any{"group": in})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleOutboundGroupItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(rest), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, http.StatusNotFound, "outbound group not found")
		return
	}
	id := parts[0]

	if len(parts) > 1 {
		if parts[1] != "summary" || r.Method != http.MethodGet {
			jsonError(w, http.StatusNotFound, "outbound group action not found")
			return
		}
		var result map[string]any
		err := s.store.Read(func(st State) error {
			group := findOutboundGroup(&st, id)
			if group == nil {
				return errors.New("outbound group not found")
			}
			result = outboundGroupSummary(st, *group)
			return nil
		})
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonWrite(w, http.StatusOK, result)
		return
	}

	switch r.Method {
	case http.MethodGet:
		var out *OutboundGroup
		_ = s.store.Read(func(st State) error {
			if group := findOutboundGroup(&st, id); group != nil {
				cp := cloneOutboundGroup(*group)
				out = &cp
			}
			return nil
		})
		if out == nil {
			jsonError(w, http.StatusNotFound, "outbound group not found")
			return
		}
		jsonWrite(w, http.StatusOK, out)

	case http.MethodPut:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		var in OutboundGroup
		if err := decodeJSON(r, &in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := normalizeOutboundGroupInput(&in); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := updateOutboundGroupTransactional(s.store, id, in, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else if err.Error() == "outbound group not found" {
				jsonError(w, http.StatusNotFound, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "update", "outbound-group:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, http.StatusForbidden, "forbidden")
			return
		}
		if err := deleteOutboundGroupTransactional(s.store, id, s.deployOutboundNode); err != nil {
			if isOutboundDeploymentError(err) {
				jsonError(w, http.StatusBadGateway, err.Error())
			} else if err.Error() == "outbound group not found" {
				jsonError(w, http.StatusNotFound, err.Error())
			} else {
				jsonError(w, http.StatusConflict, err.Error())
			}
			return
		}
		s.audit(r, "delete", "outbound-group:"+id)
		jsonWrite(w, http.StatusOK, map[string]bool{"ok": true})

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

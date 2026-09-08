// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type routingDeploymentError struct{ cause error }

func (e *routingDeploymentError) Error() string { return e.cause.Error() }
func (e *routingDeploymentError) Unwrap() error { return e.cause }

func isRoutingDeploymentError(err error) bool {
	var target *routingDeploymentError
	return errors.As(err, &target)
}

func cloneRoutingRuleValue(in RoutingRule) RoutingRule {
	out := in
	out.InboundIDs = append([]string(nil), in.InboundIDs...)
	out.UserIDs = append([]string(nil), in.UserIDs...)
	out.Domains = append([]string(nil), in.Domains...)
	out.IPs = append([]string(nil), in.IPs...)
	out.Protocols = append([]string(nil), in.Protocols...)
	return out
}

func routingDeployOrder(oldNodeID, newNodeID string) []string {
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

func redeployRoutingNodes(nodes []string, deploy func(string) error) error {
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

func createRoutingRuleTransactional(store *Store, rule RoutingRule, deploy func(string) error) error {
	if err := store.Update(func(st *State) error {
		if err := validateRoutingReferences(st, rule); err != nil {
			return err
		}
		st.RoutingRules = append(st.RoutingRules, cloneRoutingRuleValue(rule))
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(rule.NodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", rule.NodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			st.RoutingRules = deleteByID(st.RoutingRules, rule.ID, func(x RoutingRule) string { return x.ID })
			return nil
		})
		recoveryErr := deploy(rule.NodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func updateRoutingRuleTransactional(store *Store, id string, in RoutingRule, deploy func(string) error) error {
	var before RoutingRule
	var oldNodeID, newNodeID string

	if err := store.Update(func(st *State) error {
		current := findRoutingRule(st, id)
		if current == nil {
			return errors.New("routing rule not found")
		}

		in.ID = current.ID
		in.CreatedAt = current.CreatedAt
		in.UpdatedAt = time.Now().UTC()
		if err := validateRoutingReferences(st, in); err != nil {
			return err
		}

		before = cloneRoutingRuleValue(*current)
		oldNodeID = current.NodeID
		newNodeID = in.NodeID
		*current = cloneRoutingRuleValue(in)
		return nil
	}); err != nil {
		return err
	}

	nodes := routingDeployOrder(oldNodeID, newNodeID)
	for _, nodeID := range nodes {
		if err := deploy(nodeID); err != nil {
			deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
			rollbackErr := store.Update(func(st *State) error {
				current := findRoutingRule(st, id)
				if current == nil {
					return errors.New("routing rule disappeared during rollback")
				}
				*current = cloneRoutingRuleValue(before)
				return nil
			})
			recoveryErr := redeployRoutingNodes(nodes, deploy)
			switch {
			case rollbackErr != nil && recoveryErr != nil:
				return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
			case rollbackErr != nil:
				return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
			case recoveryErr != nil:
				return &routingDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
			default:
				return &routingDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
			}
		}
	}
	return nil
}

func deleteRoutingRuleTransactional(store *Store, id string, deploy func(string) error) error {
	var before RoutingRule
	var index int
	var nodeID string

	if err := store.Update(func(st *State) error {
		current := findRoutingRule(st, id)
		if current == nil {
			return errors.New("routing rule not found")
		}
		before = cloneRoutingRuleValue(*current)
		nodeID = current.NodeID
		index = -1
		for i := range st.RoutingRules {
			if st.RoutingRules[i].ID == id {
				index = i
				break
			}
		}
		st.RoutingRules = deleteByID(st.RoutingRules, id, func(x RoutingRule) string { return x.ID })
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			if findRoutingRule(st, id) != nil {
				return nil
			}
			if index < 0 || index > len(st.RoutingRules) {
				st.RoutingRules = append(st.RoutingRules, before)
				return nil
			}
			st.RoutingRules = append(st.RoutingRules, RoutingRule{})
			copy(st.RoutingRules[index+1:], st.RoutingRules[index:])
			st.RoutingRules[index] = before
			return nil
		})
		recoveryErr := deploy(nodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &routingDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

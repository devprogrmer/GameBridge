// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"time"
)

func createInboundTransactional(store *Store, obj Inbound, deploy func(string) error) error {
	if err := store.Update(func(st *State) error {
		if err := validateInboundTargetSpec(st, "", obj.Name, obj.NodeID, obj.Listen, obj.Port); err != nil {
			return err
		}
		st.Inbounds = append(st.Inbounds, obj)
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(obj.NodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", obj.NodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			st.Inbounds = deleteByID(st.Inbounds, obj.ID, func(x Inbound) string { return x.ID })
			return nil
		})
		recoveryErr := deploy(obj.NodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func setInboundEnabledTransactional(store *Store, id string, enabled bool, deploy func(string) error) error {
	var before Inbound
	var nodeID string

	if err := store.Update(func(st *State) error {
		in := findInbound(st, id)
		if in == nil {
			return errors.New("inbound not found")
		}
		if enabled {
			if err := validateInboundTarget(st, in.ID, in.NodeID, in.Listen, in.Port); err != nil {
				return err
			}
		}
		if err := validateInboundReferenceChange(st, id, in.NodeID, enabled, false); err != nil {
			return err
		}
		before = cloneInboundValue(*in)
		nodeID = in.NodeID
		in.Enabled = enabled
		if enabled {
			in.Status = "configured"
		} else {
			in.Status = "disabled"
		}
		in.UpdatedAt = time.Now().UTC()
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := restoreInboundSnapshot(store, id, before)
		recoveryErr := deploy(nodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

func deleteInboundTransactional(store *Store, id string, deploy func(string) error) error {
	var before Inbound
	var beforeClients []InboundClient
	var index int
	var nodeID string

	if err := store.Update(func(st *State) error {
		in := findInbound(st, id)
		if in == nil {
			return errors.New("inbound not found")
		}
		if err := validateInboundReferenceChange(st, id, in.NodeID, false, true); err != nil {
			return err
		}

		before = cloneInboundValue(*in)
		beforeClients = append([]InboundClient(nil), st.InboundClients...)
		nodeID = in.NodeID
		index = -1
		for i := range st.Inbounds {
			if st.Inbounds[i].ID == id {
				index = i
				break
			}
		}
		st.Inbounds = deleteByID(st.Inbounds, id, func(x Inbound) string { return x.ID })

		clients := st.InboundClients[:0]
		for _, client := range st.InboundClients {
			if client.InboundID != id {
				clients = append(clients, client)
			}
		}
		st.InboundClients = clients
		return nil
	}); err != nil {
		return err
	}

	if err := deploy(nodeID); err != nil {
		deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
		rollbackErr := store.Update(func(st *State) error {
			if findInbound(st, id) == nil {
				if index < 0 || index > len(st.Inbounds) {
					st.Inbounds = append(st.Inbounds, before)
				} else {
					st.Inbounds = append(st.Inbounds, Inbound{})
					copy(st.Inbounds[index+1:], st.Inbounds[index:])
					st.Inbounds[index] = before
				}
			}
			st.InboundClients = append([]InboundClient(nil), beforeClients...)
			return nil
		})
		recoveryErr := deploy(nodeID)
		switch {
		case rollbackErr != nil && recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v; runtime recovery failed: %v", deployFailure, rollbackErr, recoveryErr)}
		case rollbackErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rollback failed: %v", deployFailure, rollbackErr)}
		case recoveryErr != nil:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state rolled back; runtime recovery failed: %v", deployFailure, recoveryErr)}
		default:
			return &inboundDeploymentError{cause: fmt.Errorf("%v; state and runtime rolled back", deployFailure)}
		}
	}
	return nil
}

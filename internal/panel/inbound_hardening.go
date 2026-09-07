// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type inboundDeploymentError struct{ cause error }

func (e *inboundDeploymentError) Error() string { return e.cause.Error() }
func (e *inboundDeploymentError) Unwrap() error { return e.cause }

func isInboundDeploymentError(err error) bool {
	var target *inboundDeploymentError
	return errors.As(err, &target)
}

func validateInboundTargetSpec(st *State, inboundID, name, nodeID, listen string, port int) error {
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
	name = strings.TrimSpace(name)
	for _, x := range st.Inbounds {
		if x.ID == inboundID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(x.Name), name) {
			return errors.New("inbound name already exists")
		}
		if x.NodeID == nodeID && x.Port == port && x.Listen == listen {
			return errors.New("listen address/port is already used by another inbound")
		}
	}
	return nil
}

func cloneInboundValue(in Inbound) Inbound {
	out := in
	out.RealityServerNames = append([]string(nil), in.RealityServerNames...)
	out.RealityShortIDs = append([]string(nil), in.RealityShortIDs...)
	return out
}

func applyInboundInput(x *Inbound, in inboundCreate) {
	x.Name = in.Name
	x.Protocol = in.Protocol
	x.NodeID = in.NodeID
	x.Listen = in.Listen
	x.Port = in.Port
	x.Transport = in.Transport
	x.TLSMode = in.TLSMode
	x.Enabled = in.Enabled
	x.Remark = in.Remark
	x.Path = in.Path
	x.Host = in.Host
	x.ServiceName = in.ServiceName
	x.ServerName = in.ServerName
	x.CertFile = in.CertFile
	x.KeyFile = in.KeyFile
	x.RealityDest = in.RealityDest
	x.RealityServerNames = append([]string(nil), in.RealityServerNames...)
	x.RealityShortIDs = append([]string(nil), in.RealityShortIDs...)
	x.RealityFingerprint = in.RealityFingerprint
	x.ShadowsocksMethod = in.ShadowsocksMethod
	if x.TLSMode != "reality" {
		x.RealityPrivateKeyEnc = ""
		x.RealityPublicKey = ""
	}
	x.UpdatedAt = time.Now().UTC()
}

func inboundDeployOrder(oldNodeID, newNodeID string) []string {
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

func redeployInboundNodes(nodes []string, deploy func(string) error) error {
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

func restoreInboundSnapshot(store *Store, id string, before Inbound) error {
	return store.Update(func(st *State) error {
		x := findInbound(st, id)
		if x == nil {
			return errors.New("inbound disappeared during rollback")
		}
		*x = cloneInboundValue(before)
		return nil
	})
}

func updateInboundTransactional(store *Store, id string, in inboundCreate, deploy func(string) error) error {
	var before Inbound
	var oldNodeID, newNodeID string

	if err := store.Update(func(st *State) error {
		x := findInbound(st, id)
		if x == nil {
			return errors.New("inbound not found")
		}
		if err := validateInboundTargetSpec(st, id, in.Name, in.NodeID, in.Listen, in.Port); err != nil {
			return err
		}
		before = cloneInboundValue(*x)
		oldNodeID = x.NodeID
		newNodeID = in.NodeID
		applyInboundInput(x, in)
		return nil
	}); err != nil {
		return err
	}

	nodes := inboundDeployOrder(oldNodeID, newNodeID)
	for _, nodeID := range nodes {
		if err := deploy(nodeID); err != nil {
			deployFailure := fmt.Errorf("deploy node %s: %w", nodeID, err)
			rollbackErr := restoreInboundSnapshot(store, id, before)
			recoveryErr := redeployInboundNodes(nodes, deploy)
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
	}
	return nil
}

// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
)

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func validateNodeDeleteReferences(st *State, nodeID string) error {
	for _, t := range st.Tunnels {
		if t.SourceNodeID == nodeID || t.DestinationNodeID == nodeID {
			return errors.New("node has attached tunnels")
		}
	}
	for _, in := range st.Inbounds {
		if in.NodeID == nodeID {
			return errors.New("node has attached inbounds")
		}
	}
	for _, out := range st.Outbounds {
		if out.NodeID == nodeID {
			return errors.New("node has attached outbounds")
		}
	}
	for _, group := range st.OutboundGroups {
		if group.NodeID == nodeID {
			return errors.New("node has attached outbound groups")
		}
	}
	for _, rule := range st.RoutingRules {
		if rule.NodeID == nodeID {
			return errors.New("node has attached routing rules")
		}
	}
	for _, forward := range st.Forwards {
		if forward.NodeID == nodeID {
			return errors.New("node has attached port forwards")
		}
	}
	for _, peer := range st.VPNPeers {
		if peer.NodeID == nodeID {
			return errors.New("node has attached VPN peers")
		}
	}
	return nil
}

func validateInboundReferenceChange(st *State, inboundID, newNodeID string, enabled, deleting bool) error {
	for _, rule := range st.RoutingRules {
		if !stringSliceContains(rule.InboundIDs, inboundID) {
			continue
		}
		if deleting {
			return fmt.Errorf("inbound is referenced by routing rule %q", rule.Name)
		}
		if rule.NodeID != newNodeID {
			return fmt.Errorf("inbound is referenced by routing rule %q on node %s", rule.Name, rule.NodeID)
		}
		if !enabled && rule.Enabled {
			return fmt.Errorf("inbound is used by enabled routing rule %q", rule.Name)
		}
	}
	return nil
}

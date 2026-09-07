// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"testing"
	"time"
)

func TestValidateInboundTarget(t *testing.T) {
	st := State{
		Nodes: []Node{
			{ID: "n1", Enabled: true, Status: NodeStatusOnline},
			{ID: "n2", Enabled: false, Status: NodeStatusDisabled},
			{ID: "n3", Enabled: true, Maintenance: true, Status: NodeStatusMaintenance},
		},
		Inbounds: []Inbound{
			{ID: "a", NodeID: "n1", Listen: "0.0.0.0", Port: 443},
		},
	}
	if err := validateInboundTarget(&st, "b", "n1", "0.0.0.0", 443); err == nil {
		t.Fatal("expected duplicate listen/port rejection")
	}
	if err := validateInboundTarget(&st, "a", "n1", "0.0.0.0", 443); err != nil {
		t.Fatalf("same inbound update should be allowed: %v", err)
	}
	if err := validateInboundTarget(&st, "x", "n2", "0.0.0.0", 8443); err == nil {
		t.Fatal("expected disabled node rejection")
	}
	if err := validateInboundTarget(&st, "x", "n3", "0.0.0.0", 8443); err == nil {
		t.Fatal("expected maintenance node rejection")
	}
}

func TestInboundSummaryLifecycleAware(t *testing.T) {
	now := time.Now().UTC()
	st := State{
		Nodes:    []Node{{ID: "n1", Enabled: true, Status: NodeStatusOnline}},
		Inbounds: []Inbound{{ID: "i1", NodeID: "n1", Enabled: true}},
		Users: []User{
			{ID: "u1", Status: UserStatusActive},
			{ID: "u2", Status: UserStatusDisabled},
		},
		InboundClients: []InboundClient{
			{InboundID: "i1", UserID: "u1", Enabled: true},
			{InboundID: "i1", UserID: "u2", Enabled: true},
			{InboundID: "i1", UserID: "u3", Enabled: false},
		},
	}
	got, err := inboundSummary(&st, "i1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.AttachedUsers != 3 || got.EnabledUsers != 2 || got.ActiveUsers != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if got.NodeStatus != NodeStatusOnline || !got.NodeEnabled {
		t.Fatalf("unexpected node state: %+v", got)
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStoreForFinishing(t *testing.T, st State) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	store := &Store{path: path, st: cloneState(st)}
	if store.st.Settings == nil {
		store.st.Settings = map[string]string{"site_name": "GameBridge"}
	}
	return store
}

func TestStoreUpdateCallbackFailureDoesNotMutateState(t *testing.T) {
	store := testStoreForFinishing(t, State{
		Settings: map[string]string{"site_name": "before"},
	})
	wantErr := errors.New("stop")
	if err := store.Update(func(st *State) error {
		st.Settings["site_name"] = "after"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("expected callback error, got %v", err)
	}

	var got string
	_ = store.Read(func(st State) error {
		got = st.Settings["site_name"]
		return nil
	})
	if got != "before" {
		t.Fatalf("callback failure mutated live state: %q", got)
	}
}

func TestStoreUpdateSaveFailureDoesNotMutateState(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	store := &Store{
		path: filepath.Join(blocker, "state.json"),
		st:   State{Settings: map[string]string{"site_name": "before"}},
	}
	err := store.Update(func(st *State) error {
		st.Settings["site_name"] = "after"
		return nil
	})
	if err == nil {
		t.Fatal("expected save failure")
	}

	var got string
	_ = store.Read(func(st State) error {
		got = st.Settings["site_name"]
		return nil
	})
	if got != "before" {
		t.Fatalf("save failure mutated live state: %q", got)
	}
}

func TestCloneStateDeepCopiesNewNestedSlices(t *testing.T) {
	original := State{
		Nodes: []Node{{ID: "node-1", Tags: []string{"edge"}}},
		Outbounds: []Outbound{{
			ID:                  "out-1",
			WireGuardAllowedIPs: []string{"0.0.0.0/0"},
		}},
		OutboundGroups: []OutboundGroup{{
			ID: "group-1",
			Members: []OutboundGroupMember{{
				OutboundID: "out-1",
				Weight:     1,
			}},
		}},
	}
	cloned := cloneState(original)
	cloned.Nodes[0].Tags[0] = "changed"
	cloned.Outbounds[0].WireGuardAllowedIPs[0] = "10.0.0.0/8"
	cloned.OutboundGroups[0].Members[0].Weight = 99

	if original.Nodes[0].Tags[0] != "edge" {
		t.Fatal("node tags were shallow copied")
	}
	if original.Outbounds[0].WireGuardAllowedIPs[0] != "0.0.0.0/0" {
		t.Fatal("WireGuard allowed IPs were shallow copied")
	}
	if original.OutboundGroups[0].Members[0].Weight != 1 {
		t.Fatal("outbound group members were shallow copied")
	}
}

func TestNodeDeleteGuardCoversControlPlaneResources(t *testing.T) {
	base := State{
		Nodes: []Node{{ID: "node-1", Enabled: true}},
	}
	cases := []struct {
		name string
		edit func(*State)
		want string
	}{
		{"inbound", func(st *State) { st.Inbounds = []Inbound{{ID: "in-1", NodeID: "node-1"}} }, "inbounds"},
		{"outbound", func(st *State) { st.Outbounds = []Outbound{{ID: "out-1", NodeID: "node-1"}} }, "outbounds"},
		{"group", func(st *State) { st.OutboundGroups = []OutboundGroup{{ID: "g-1", NodeID: "node-1"}} }, "groups"},
		{"routing", func(st *State) { st.RoutingRules = []RoutingRule{{ID: "r-1", NodeID: "node-1"}} }, "routing"},
		{"forward", func(st *State) { st.Forwards = []PortForward{{ID: "f-1", NodeID: "node-1"}} }, "forwards"},
		{"peer", func(st *State) { st.VPNPeers = []VPNPeer{{ID: "p-1", NodeID: "node-1"}} }, "peers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := cloneState(base)
			tc.edit(&st)
			err := validateNodeDeleteReferences(&st, "node-1")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q guard, got %v", tc.want, err)
			}
		})
	}
}

func TestInboundReferenceGuardsMoveDisableAndDelete(t *testing.T) {
	st := State{
		RoutingRules: []RoutingRule{{
			ID:         "route-1",
			Name:       "game route",
			NodeID:     "node-a",
			Enabled:    true,
			InboundIDs: []string{"in-1"},
		}},
	}
	if err := validateInboundReferenceChange(&st, "in-1", "node-b", true, false); err == nil {
		t.Fatal("expected node move to be rejected")
	}
	if err := validateInboundReferenceChange(&st, "in-1", "node-a", false, false); err == nil {
		t.Fatal("expected disabling routed inbound to be rejected")
	}
	if err := validateInboundReferenceChange(&st, "in-1", "node-a", false, true); err == nil {
		t.Fatal("expected deleting referenced inbound to be rejected")
	}
}

func TestCreateRoutingRollbackOnDeployFailure(t *testing.T) {
	st := State{
		Settings: map[string]string{"site_name": "GameBridge"},
		Nodes: []Node{{
			ID:      "node-1",
			Enabled: true,
			Status:  NodeStatusOnline,
		}},
	}
	store := testStoreForFinishing(t, st)
	rule := RoutingRule{
		ID:         "route-1",
		Name:       "route",
		NodeID:     "node-1",
		OutboundID: "direct",
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	calls := 0
	err := createRoutingRuleTransactional(store, rule, func(string) error {
		calls++
		return errors.New("deploy failed")
	})
	if err == nil || !isRoutingDeploymentError(err) {
		t.Fatalf("expected routing deployment error, got %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected deploy plus runtime recovery, got %d calls", calls)
	}
	_ = store.Read(func(st State) error {
		if len(st.RoutingRules) != 0 {
			t.Fatalf("routing rule remained after rollback: %+v", st.RoutingRules)
		}
		return nil
	})
}

func TestCreateInboundRollbackOnDeployFailure(t *testing.T) {
	st := State{
		Settings: map[string]string{"site_name": "GameBridge"},
		Nodes: []Node{{
			ID:      "node-1",
			Name:    "node",
			Enabled: true,
			Status:  NodeStatusOnline,
		}},
	}
	store := testStoreForFinishing(t, st)
	in := Inbound{
		ID:        "in-1",
		Name:      "test",
		NodeID:    "node-1",
		Listen:    "0.0.0.0",
		Port:      443,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err := createInboundTransactional(store, in, func(string) error {
		return errors.New("deploy failed")
	})
	if err == nil || !isInboundDeploymentError(err) {
		t.Fatalf("expected inbound deployment error, got %v", err)
	}
	_ = store.Read(func(st State) error {
		if len(st.Inbounds) != 0 {
			t.Fatalf("inbound remained after rollback: %+v", st.Inbounds)
		}
		return nil
	})
}

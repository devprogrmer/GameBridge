// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func hardeningTestInput(name, node string, port int) inboundCreate {
	return inboundCreate{
		Name: name, Protocol: "vless", NodeID: node, Listen: "0.0.0.0",
		Port: port, Transport: "tcp", TLSMode: "none", Enabled: true,
		Path: "/", ServiceName: "gamebridge",
	}
}

func TestInboundHardeningValidation(t *testing.T) {
	st := State{
		Nodes: []Node{
			{ID: "n1", Enabled: true, Status: NodeStatusOnline},
			{ID: "n2", Enabled: false, Status: NodeStatusDisabled},
			{ID: "n3", Enabled: true, Maintenance: true, Status: NodeStatusMaintenance},
		},
		Inbounds: []Inbound{{ID: "i1", Name: "Primary", NodeID: "n1", Listen: "0.0.0.0", Port: 443}},
	}
	cases := []inboundCreate{
		hardeningTestInput("Other", "missing", 8443),
		hardeningTestInput("Other", "n2", 8443),
		hardeningTestInput("Other", "n3", 8443),
		hardeningTestInput("primary", "n1", 8443),
		hardeningTestInput("Other", "n1", 443),
	}
	for i, in := range cases {
		if err := validateInboundTargetSpec(&st, "i2", in.Name, in.NodeID, in.Listen, in.Port); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
	if err := validateInboundTargetSpec(&st, "i1", "Primary", "n1", "0.0.0.0", 443); err != nil {
		t.Fatalf("self update must be allowed: %v", err)
	}
}

func newHardeningStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(st *State) error {
		st.Nodes = []Node{
			{ID: "n1", Enabled: true, Status: NodeStatusOnline},
			{ID: "n2", Enabled: true, Status: NodeStatusOnline},
		}
		st.Inbounds = []Inbound{{
			ID: "i1", Name: "Primary", Protocol: "vless", NodeID: "n1",
			Listen: "0.0.0.0", Port: 443, Transport: "tcp", TLSMode: "none", Enabled: true,
		}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestInboundMoveDeploysOldThenNew(t *testing.T) {
	s := newHardeningStore(t)
	var got []string
	err := updateInboundTransactional(s, "i1", hardeningTestInput("Primary", "n2", 8443), func(id string) error {
		got = append(got, id)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"n1", "n2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deploy order got %#v want %#v", got, want)
	}
	_ = s.Read(func(st State) error {
		in := findInbound(&st, "i1")
		if in == nil || in.NodeID != "n2" || in.Port != 8443 {
			t.Fatalf("move not persisted: %+v", in)
		}
		return nil
	})
}

func TestInboundMoveRollback(t *testing.T) {
	s := newHardeningStore(t)
	var got []string
	failed := false
	err := updateInboundTransactional(s, "i1", hardeningTestInput("Moved", "n2", 8443), func(id string) error {
		got = append(got, id)
		if id == "n2" && !failed {
			failed = true
			return errors.New("synthetic failure")
		}
		return nil
	})
	if err == nil || !isInboundDeploymentError(err) {
		t.Fatalf("expected deployment rollback error, got %v", err)
	}
	if want := []string{"n1", "n2", "n1", "n2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deploy/recovery order got %#v want %#v", got, want)
	}
	_ = s.Read(func(st State) error {
		in := findInbound(&st, "i1")
		if in == nil || in.NodeID != "n1" || in.Name != "Primary" || in.Port != 443 {
			t.Fatalf("rollback failed: %+v", in)
		}
		return nil
	})
}

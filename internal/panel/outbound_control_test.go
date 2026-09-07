// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func newOutboundControlStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(st *State) error {
		st.Nodes = []Node{
			{ID: "n1", Enabled: true, Status: NodeStatusOnline},
			{ID: "n2", Enabled: true, Status: NodeStatusOnline},
			{ID: "disabled", Enabled: false, Status: NodeStatusDisabled},
			{ID: "maintenance", Enabled: true, Maintenance: true, Status: NodeStatusMaintenance},
		}
		st.Outbounds = []Outbound{{
			ID: "o1", Name: "Primary", NodeID: "n1", Tag: "primary",
			Protocol: "socks", Address: "127.0.0.1", Port: 1080, Enabled: true,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOutboundTargetValidation(t *testing.T) {
	s := newOutboundControlStore(t)
	var st State
	_ = s.Read(func(x State) error { st = x; return nil })

	cases := []struct {
		name, node, tag string
	}{
		{"Other", "missing", "other"},
		{"Other", "disabled", "other"},
		{"Other", "maintenance", "other"},
		{"Primary", "n1", "other"},
		{"Other", "n1", "primary"},
	}
	for i, tc := range cases {
		if err := validateOutboundTargetSpec(&st, "o2", tc.name, tc.node, tc.tag); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
	if err := validateOutboundTargetSpec(&st, "o1", "Primary", "n1", "primary"); err != nil {
		t.Fatalf("self update must be allowed: %v", err)
	}
}

func TestOutboundCreateRollback(t *testing.T) {
	s := newOutboundControlStore(t)
	obj := Outbound{
		ID: "o2", Name: "Backup", NodeID: "n1", Tag: "backup",
		Protocol: "http", Address: "127.0.0.1", Port: 8080, Enabled: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	calls := 0
	err := createOutboundTransactional(s, obj, func(string) error {
		calls++
		if calls == 1 {
			return errors.New("synthetic failure")
		}
		return nil
	})
	if err == nil || !isOutboundDeploymentError(err) {
		t.Fatalf("expected deployment rollback error, got %v", err)
	}
	_ = s.Read(func(st State) error {
		if findOutbound(&st, "o2") != nil {
			t.Fatal("failed create must be removed from state")
		}
		return nil
	})
}

func TestOutboundMoveRollback(t *testing.T) {
	s := newOutboundControlStore(t)
	in := outboundInput{
		Name: "Moved", NodeID: "n2", Tag: "moved", Protocol: "socks",
		Address: "127.0.0.1", Port: 1081, Enabled: true,
	}
	var got []string
	failed := false
	err := updateOutboundTransactional(s, "o1", in, "", func(id string) error {
		got = append(got, id)
		if id == "n2" && !failed {
			failed = true
			return errors.New("synthetic failure")
		}
		return nil
	})
	if err == nil || !isOutboundDeploymentError(err) {
		t.Fatalf("expected deployment rollback error, got %v", err)
	}
	if want := []string{"n1", "n2", "n1", "n2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deploy/recovery order got %#v want %#v", got, want)
	}
	_ = s.Read(func(st State) error {
		x := findOutbound(&st, "o1")
		if x == nil || x.NodeID != "n1" || x.Name != "Primary" || x.Tag != "primary" {
			t.Fatalf("rollback failed: %+v", x)
		}
		return nil
	})
}

func TestOutboundDisableGuardAndRollback(t *testing.T) {
	s := newOutboundControlStore(t)
	if err := s.Update(func(st *State) error {
		st.RoutingRules = append(st.RoutingRules, RoutingRule{
			ID: "r1", Name: "route", NodeID: "n1", OutboundID: "o1", Enabled: true,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := setOutboundEnabledTransactional(s, "o1", false, func(string) error { return nil }); err == nil {
		t.Fatal("expected disable to be blocked while an enabled rule references outbound")
	}

	if err := s.Update(func(st *State) error {
		st.RoutingRules[0].Enabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	err := setOutboundEnabledTransactional(s, "o1", false, func(string) error {
		calls++
		if calls == 1 {
			return errors.New("synthetic failure")
		}
		return nil
	})
	if err == nil || !isOutboundDeploymentError(err) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	_ = s.Read(func(st State) error {
		x := findOutbound(&st, "o1")
		if x == nil || !x.Enabled {
			t.Fatalf("enabled state was not restored: %+v", x)
		}
		return nil
	})
}

func TestOutboundDeleteRollback(t *testing.T) {
	s := newOutboundControlStore(t)
	calls := 0
	err := deleteOutboundTransactional(s, "o1", func(string) error {
		calls++
		if calls == 1 {
			return errors.New("synthetic failure")
		}
		return nil
	})
	if err == nil || !isOutboundDeploymentError(err) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	_ = s.Read(func(st State) error {
		x := findOutbound(&st, "o1")
		if x == nil || x.Name != "Primary" {
			t.Fatalf("deleted outbound was not restored: %+v", x)
		}
		return nil
	})
}

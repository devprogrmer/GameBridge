// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNodeStatusAfterProbe(t *testing.T) {
	cases := []struct {
		enabled     bool
		maintenance bool
		failures    int
		want        string
	}{
		{false, false, 0, NodeStatusDisabled},
		{true, true, 0, NodeStatusMaintenance},
		{true, false, 0, NodeStatusOnline},
		{true, false, 1, NodeStatusDegraded},
		{true, false, 2, NodeStatusDegraded},
		{true, false, 3, NodeStatusOffline},
	}
	for _, c := range cases {
		if got := nodeStatusAfterProbe(c.enabled, c.maintenance, c.failures); got != c.want {
			t.Fatalf("got %q, want %q", got, c.want)
		}
	}
}

func TestValidateNodeForWork(t *testing.T) {
	if err := validateNodeForWork(Node{Name: "ok", Enabled: true, Status: NodeStatusOnline}); err != nil {
		t.Fatalf("online node rejected: %v", err)
	}
	for _, n := range []Node{
		{Name: "disabled", Status: NodeStatusDisabled},
		{Name: "maintenance", Enabled: true, Maintenance: true, Status: NodeStatusMaintenance},
		{Name: "degraded", Enabled: true, Status: NodeStatusDegraded},
		{Name: "offline", Enabled: true, Status: NodeStatusOffline},
		{Name: "unknown", Enabled: true, Status: NodeStatusUnknown},
	} {
		if err := validateNodeForWork(n); err == nil {
			t.Fatalf("node %q should be rejected", n.Name)
		}
	}
}

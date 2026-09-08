// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func balancingFixtureState() State {
	return State{
		Nodes: []Node{
			{ID: "node-1", Name: "node-1", Enabled: true, Status: NodeStatusOnline},
		},
		Outbounds: []Outbound{
			{ID: "out-a", Name: "A", NodeID: "node-1", Tag: "proxy-a", Protocol: "freedom", Enabled: true, HealthStatus: "healthy"},
			{ID: "out-b", Name: "B", NodeID: "node-1", Tag: "proxy-b", Protocol: "freedom", Enabled: true, HealthStatus: "healthy"},
			{ID: "out-c", Name: "C", NodeID: "node-1", Tag: "proxy-c", Protocol: "freedom", Enabled: true, HealthStatus: "healthy"},
		},
	}
}

func TestNormalizeOutboundGroupInputDefaultsFailClosed(t *testing.T) {
	group := OutboundGroup{
		Name:   "Primary",
		NodeID: "node-1",
		Members: []OutboundGroupMember{
			{OutboundID: "out-a"},
			{OutboundID: "out-b", Priority: 10, Weight: 5},
		},
	}
	if err := normalizeOutboundGroupInput(&group); err != nil {
		t.Fatal(err)
	}
	if group.Strategy != outboundGroupStrategyLeastPing {
		t.Fatalf("unexpected default strategy %q", group.Strategy)
	}
	if group.FallbackOutboundID != "blocked" {
		t.Fatalf("expected fail-closed fallback, got %q", group.FallbackOutboundID)
	}
	if group.Expected != 1 {
		t.Fatalf("unexpected expected=%d", group.Expected)
	}
	if group.Members[0].Weight != 1 {
		t.Fatalf("expected default member weight 1, got %d", group.Members[0].Weight)
	}
}

func TestValidateOutboundGroupSpecAndReferenceIntegrity(t *testing.T) {
	st := balancingFixtureState()
	group := OutboundGroup{
		ID:                 "group-1",
		Name:               "Primary",
		NodeID:             "node-1",
		Strategy:           outboundGroupStrategyLeastPing,
		FallbackOutboundID: "out-c",
		Expected:           1,
		Enabled:            true,
		Members: []OutboundGroupMember{
			{OutboundID: "out-a", Weight: 1},
			{OutboundID: "out-b", Weight: 1},
		},
	}
	if err := validateOutboundGroupSpec(&st, group); err != nil {
		t.Fatal(err)
	}
	st.OutboundGroups = []OutboundGroup{group}
	if err := validateOutboundGroupReferencesForChange(&st, "out-a", "node-1", false); err == nil {
		t.Fatal("expected disabling referenced member to be rejected")
	}
	if err := validateOutboundGroupReferencesForChange(&st, "out-c", "node-1", false); err == nil {
		t.Fatal("expected disabling referenced fallback to be rejected")
	}
}

func TestBuildXrayBalancerAliasesAndBalancer(t *testing.T) {
	st := balancingFixtureState()
	group := OutboundGroup{
		ID:                 "group-1",
		Name:               "Primary",
		NodeID:             "node-1",
		Strategy:           outboundGroupStrategyRoundRobin,
		FallbackOutboundID: "blocked",
		Expected:           1,
		Enabled:            true,
		Members: []OutboundGroupMember{
			{OutboundID: "out-a", Weight: 1},
			{OutboundID: "out-b", Weight: 1},
		},
	}
	st.OutboundGroups = []OutboundGroup{group}

	base := []any{
		map[string]any{"tag": "direct", "protocol": "freedom", "settings": map[string]any{}},
		map[string]any{"tag": "blocked", "protocol": "blackhole", "settings": map[string]any{}},
		map[string]any{"tag": "proxy-a", "protocol": "freedom", "settings": map[string]any{}},
		map[string]any{"tag": "proxy-b", "protocol": "freedom", "settings": map[string]any{}},
		map[string]any{"tag": "proxy-c", "protocol": "freedom", "settings": map[string]any{}},
	}
	aliases, err := buildXrayBalancerAliases(st, "node-1", base)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}

	balancers := buildXrayBalancers(st, "node-1")
	if len(balancers) != 1 {
		t.Fatalf("expected one balancer, got %d", len(balancers))
	}
	balancer := balancers[0].(map[string]any)
	if balancer["fallbackTag"] != "blocked" {
		t.Fatalf("expected blocked fallback, got %+v", balancer)
	}
	strategy := balancer["strategy"].(map[string]any)
	if strategy["type"] != "roundRobin" {
		t.Fatalf("unexpected strategy: %+v", strategy)
	}
}

func TestLeastLoadWeightsAndPrioritiesBecomeCosts(t *testing.T) {
	group := OutboundGroup{
		ID:       "group-1",
		Strategy: outboundGroupStrategyLeastLoad,
		Expected: 1,
		Members: []OutboundGroupMember{
			{OutboundID: "out-a", Priority: 0, Weight: 10},
			{OutboundID: "out-b", Priority: 10, Weight: 1},
		},
	}
	strategy := xrayOutboundGroupStrategy(group)
	settings := strategy["settings"].(map[string]any)
	costs := settings["costs"].([]any)
	first := costs[0].(map[string]any)["value"].(float64)
	second := costs[1].(map[string]any)["value"].(float64)
	if first >= second {
		t.Fatalf("expected higher-priority/higher-weight member to have lower cost: first=%v second=%v", first, second)
	}
}

func TestRoutingRuleCanTargetOutboundGroup(t *testing.T) {
	st := balancingFixtureState()
	group := OutboundGroup{
		ID:                 "group-1",
		Name:               "Primary",
		NodeID:             "node-1",
		Strategy:           outboundGroupStrategyLeastPing,
		FallbackOutboundID: "blocked",
		Expected:           1,
		Enabled:            true,
		Members: []OutboundGroupMember{
			{OutboundID: "out-a", Weight: 1},
			{OutboundID: "out-b", Weight: 1},
		},
	}
	st.OutboundGroups = []OutboundGroup{group}
	rule := RoutingRule{
		ID:              "route-1",
		Name:            "group route",
		NodeID:          "node-1",
		Enabled:         true,
		OutboundGroupID: "group-1",
	}
	if err := normalizeRoutingRule(&rule); err != nil {
		t.Fatal(err)
	}
	if err := validateRoutingReferences(&st, rule); err != nil {
		t.Fatal(err)
	}
	st.RoutingRules = []RoutingRule{rule}
	routing := buildXrayRouting(st, "node-1")
	rules := routing["rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected one routing rule, got %d", len(rules))
	}
	got := rules[0].(map[string]any)
	if got["balancerTag"] != outboundGroupBalancerTag("group-1") {
		t.Fatalf("routing rule did not target balancer: %+v", got)
	}
}

func TestOutboundGroupsPersistThroughDiskState(t *testing.T) {
	st := balancingFixtureState()
	st.OutboundGroups = []OutboundGroup{{
		ID:                 "group-1",
		Name:               "Primary",
		NodeID:             "node-1",
		Strategy:           outboundGroupStrategyLeastPing,
		FallbackOutboundID: "blocked",
		Expected:           1,
		Enabled:            true,
		Members: []OutboundGroupMember{
			{OutboundID: "out-a", Weight: 1},
		},
	}}
	roundTrip := diskToState(stateToDisk(st))
	if len(roundTrip.OutboundGroups) != 1 || roundTrip.OutboundGroups[0].ID != "group-1" {
		t.Fatalf("outbound groups were not persisted: %+v", roundTrip.OutboundGroups)
	}
}

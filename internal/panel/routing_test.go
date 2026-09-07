// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestBuildXrayRouting(t *testing.T) {
	st := State{
		Users:     []User{{ID: "u1", Username: "alice"}},
		Inbounds:  []Inbound{{ID: "i1", NodeID: "n1"}},
		Outbounds: []Outbound{{ID: "o1", NodeID: "n1", Tag: "proxy-a", Enabled: true}},
		RoutingRules: []RoutingRule{
			{
				ID: "r2", Name: "second", NodeID: "n1", Priority: 200, Enabled: true,
				Domains: []string{"domain:example.com"}, OutboundID: "direct",
			},
			{
				ID: "r1", Name: "first", NodeID: "n1", Priority: 100, Enabled: true,
				InboundIDs: []string{"i1"}, UserIDs: []string{"u1"}, OutboundID: "o1",
			},
		},
	}
	cfg := buildXrayRouting(st, "n1")
	rules, ok := cfg["rules"].([]any)
	if !ok || len(rules) != 2 {
		t.Fatalf("unexpected rules: %#v", cfg["rules"])
	}
	first := rules[0].(map[string]any)
	if first["outboundTag"] != "proxy-a" {
		t.Fatalf("priority order or outbound resolution failed: %#v", first)
	}
	users := first["user"].([]string)
	if len(users) != 1 || users[0] != "gb:alice" {
		t.Fatalf("unexpected user routing match: %#v", users)
	}
}

func TestNormalizeOutboundInput(t *testing.T) {
	x := outboundInput{Name: "Proxy", NodeID: "n", Tag: "proxy-1", Protocol: "socks", Address: "127.0.0.1", Port: 1080}
	if err := normalizeOutboundInput(&x); err != nil {
		t.Fatal(err)
	}
	bad := outboundInput{Name: "Bad", NodeID: "n", Tag: "direct", Protocol: "freedom"}
	if err := normalizeOutboundInput(&bad); err == nil {
		t.Fatal("reserved tag should be rejected")
	}
}

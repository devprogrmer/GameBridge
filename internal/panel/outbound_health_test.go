// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"testing"
	"time"
)

func TestBuildOutboundHealthProbesDeterministicAndAvoidsInboundPorts(t *testing.T) {
	st := State{
		Inbounds: []Inbound{
			{ID: "in-1", NodeID: "node-1", Port: outboundHealthPortMin, Enabled: true},
		},
		Outbounds: []Outbound{
			{ID: "b", NodeID: "node-1", Tag: "b-tag", Protocol: "freedom", Enabled: true},
			{ID: "a", NodeID: "node-1", Tag: "a-tag", Protocol: "socks", Enabled: true},
			{ID: "c", NodeID: "node-1", Tag: "blocked", Protocol: "blackhole", Enabled: true},
			{ID: "d", NodeID: "node-1", Tag: "disabled", Protocol: "freedom", Enabled: false},
		},
	}

	probes, err := buildOutboundHealthProbes(st, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 2 {
		t.Fatalf("expected 2 probes, got %d", len(probes))
	}
	if probes[0].OutboundID != "a" || probes[1].OutboundID != "b" {
		t.Fatalf("expected deterministic ID ordering, got %+v", probes)
	}
	if probes[0].Port != outboundHealthPortMin+1 {
		t.Fatalf("expected occupied inbound port to be skipped, got %d", probes[0].Port)
	}
}

func TestBuildOutboundHealthXrayUsesLocalSocksAndDirectOutboundRule(t *testing.T) {
	st := State{
		Outbounds: []Outbound{
			{ID: "out-1", NodeID: "node-1", Tag: "proxy-one", Protocol: "vless", Enabled: true},
		},
	}
	inbounds, rules, err := buildOutboundHealthXray(st, "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 1 || len(rules) != 1 {
		t.Fatalf("expected one health inbound and rule, got %d/%d", len(inbounds), len(rules))
	}

	inbound := inbounds[0].(map[string]any)
	if inbound["listen"] != "127.0.0.1" || inbound["protocol"] != "socks" {
		t.Fatalf("unexpected health inbound: %+v", inbound)
	}
	settings := inbound["settings"].(map[string]any)
	if settings["auth"] != "noauth" || settings["udp"] != false {
		t.Fatalf("unexpected SOCKS settings: %+v", settings)
	}

	rule := rules[0].(map[string]any)
	if rule["outboundTag"] != "proxy-one" {
		t.Fatalf("health rule points to wrong outbound: %+v", rule)
	}
}

func TestApplyOutboundHealthResult(t *testing.T) {
	out := Outbound{ID: "out-1", HealthFailureCount: 2}
	checked := time.Date(2026, 9, 8, 1, 2, 3, 0, time.UTC)

	applyOutboundHealthResult(&out, outboundProbeResponse{
		Healthy:   true,
		LatencyMS: 123,
		CheckedAt: checked,
		Target:    "https://example.test",
	})
	if out.HealthStatus != "healthy" || out.HealthLatencyMS != 123 || out.HealthFailureCount != 0 {
		t.Fatalf("unexpected healthy state: %+v", out)
	}
	if out.HealthLastSuccessAt == nil || !out.HealthLastSuccessAt.Equal(checked) {
		t.Fatalf("last success not stored: %+v", out.HealthLastSuccessAt)
	}

	applyOutboundHealthResult(&out, outboundProbeResponse{
		Healthy:   false,
		CheckedAt: checked.Add(time.Minute),
		Error:     "timeout",
	})
	if out.HealthStatus != "unhealthy" || out.HealthLatencyMS != 0 || out.HealthLastError != "timeout" || out.HealthFailureCount != 1 {
		t.Fatalf("unexpected unhealthy state: %+v", out)
	}
}

func TestNormalizeOutboundHealthStatusDefaultsUnknown(t *testing.T) {
	if got := normalizeOutboundHealthStatus(""); got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

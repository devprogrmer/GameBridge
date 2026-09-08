// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeCustomXrayConfigCanonicalizesAllowedOutbound(t *testing.T) {
	raw := `{
		"protocol":"hysteria",
		"settings":{"address":"vpn.example","port":443},
		"streamSettings":{"sockopt":{"mark":12345,"interface":"eth0"}},
		"mux":{"enabled":false},
		"targetStrategy":"UseIPv4"
	}`
	obj, protocol, canonical, err := decodeCustomXrayConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if protocol != "hysteria" {
		t.Fatalf("unexpected protocol %q", protocol)
	}
	if obj["protocol"] != "hysteria" {
		t.Fatalf("unexpected decoded object: %+v", obj)
	}

	var roundTrip map[string]any
	if err := json.Unmarshal([]byte(canonical), &roundTrip); err != nil {
		t.Fatalf("canonical output is not valid JSON: %v", err)
	}
}

func TestCustomXrayOutboundItemOwnsTag(t *testing.T) {
	item, protocol, err := customXrayOutboundItem("custom-main", `{"protocol":"freedom","settings":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if protocol != "freedom" || item["tag"] != "custom-main" {
		t.Fatalf("unexpected custom outbound: %+v protocol=%s", item, protocol)
	}
}

func TestDecodeCustomXrayConfigRejectsManagedDependencyFields(t *testing.T) {
	bad := []string{
		`{"protocol":"freedom","tag":"evil","settings":{}}`,
		`{"protocol":"freedom","proxySettings":{"tag":"direct"}}`,
		`{"protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"direct"}}}`,
	}
	for _, raw := range bad {
		if _, _, _, err := decodeCustomXrayConfig(raw); err == nil {
			t.Fatalf("expected config to be rejected: %s", raw)
		}
	}
}

func TestDecodeCustomXrayConfigRejectsUnknownTopLevelAndBadTypes(t *testing.T) {
	bad := []string{
		`{"protocol":"freedom","inbounds":[]}`,
		`{"protocol":"freedom","settings":[]}`,
		`{"protocol":"freedom","sendThrough":"not-an-ip"}`,
		`{"protocol":"freedom","targetStrategy":"whatever"}`,
		`{"protocol":"Freedom"}`,
	}
	for _, raw := range bad {
		if _, _, _, err := decodeCustomXrayConfig(raw); err == nil {
			t.Fatalf("expected config to be rejected: %s", raw)
		}
	}
}

func TestNormalizeCustomOutboundInput(t *testing.T) {
	in := outboundInput{
		Name:     "Advanced",
		NodeID:   "node-1",
		Tag:      "advanced-main",
		Protocol: "custom",
		Secret:   "{\n  \"protocol\": \"hysteria\", \"settings\": {} \n}",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.CustomXrayProtocol != "hysteria" {
		t.Fatalf("unexpected custom protocol %q", in.CustomXrayProtocol)
	}
	if strings.Contains(in.Secret, "\n") {
		t.Fatalf("expected canonical compact JSON, got %q", in.Secret)
	}
	if !outboundProtocolNeedsSecret("custom") {
		t.Fatal("custom outbound must require encrypted custom config")
	}
}

func TestCustomXrayConfigSizeLimit(t *testing.T) {
	raw := `{"protocol":"freedom","settings":{"padding":"` + strings.Repeat("x", customXrayConfigMaxBytes) + `"}}`
	if _, _, _, err := decodeCustomXrayConfig(raw); err == nil {
		t.Fatal("expected oversized custom config to be rejected")
	}
}

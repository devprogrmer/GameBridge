// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNormalizeOutboundVLESSReality(t *testing.T) {
	in := outboundInput{
		Name:             "edge-vless",
		NodeID:           "n1",
		Tag:              "edge-vless",
		Protocol:         "vless",
		Address:          "edge.example.com",
		Port:             443,
		Secret:           "11111111-1111-4111-8111-111111111111",
		Transport:        "raw",
		TLSMode:          "reality",
		RealityPublicKey: "server-public-key",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if in.ServerName != "edge.example.com" {
		t.Fatalf("server name default = %q", in.ServerName)
	}
	if in.Fingerprint != "chrome" {
		t.Fatalf("fingerprint default = %q", in.Fingerprint)
	}
}

func TestNormalizeOutboundRejectsRealityWebsocket(t *testing.T) {
	in := outboundInput{
		Name:             "bad",
		NodeID:           "n1",
		Tag:              "bad",
		Protocol:         "vless",
		Address:          "edge.example.com",
		Port:             443,
		Secret:           "11111111-1111-4111-8111-111111111111",
		Transport:        "ws",
		TLSMode:          "reality",
		RealityPublicKey: "server-public-key",
	}
	if err := normalizeOutboundInput(&in); err == nil {
		t.Fatal("expected websocket + REALITY to be rejected")
	}
}

func TestNormalizeOutboundShadowsocksDefaults(t *testing.T) {
	in := outboundInput{
		Name:     "ss",
		NodeID:   "n1",
		Tag:      "ss",
		Protocol: "shadowsocks",
		Address:  "127.0.0.1",
		Port:     8388,
		Secret:   "long-enough-password",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if in.ShadowsocksMethod != "aes-128-gcm" {
		t.Fatalf("method = %q", in.ShadowsocksMethod)
	}
	if in.Transport != "raw" {
		t.Fatalf("transport = %q", in.Transport)
	}
}

func TestBuildXrayOutboundTLSStream(t *testing.T) {
	stream := buildXrayOutboundStream(Outbound{
		Address:       "proxy.example.com",
		Transport:     "ws",
		TLSMode:       "tls",
		Path:          "/edge",
		Host:          "cdn.example.com",
		ServerName:    "sni.example.com",
		AllowInsecure: false,
		Fingerprint:   "chrome",
	})
	if stream["method"] != "websocket" {
		t.Fatalf("method = %#v", stream["method"])
	}
	if stream["security"] != "tls" {
		t.Fatalf("security = %#v", stream["security"])
	}
	ws, ok := stream["wsSettings"].(map[string]any)
	if !ok || ws["path"] != "/edge" || ws["host"] != "cdn.example.com" {
		t.Fatalf("wsSettings = %#v", stream["wsSettings"])
	}
	tls, ok := stream["tlsSettings"].(map[string]any)
	if !ok || tls["serverName"] != "sni.example.com" {
		t.Fatalf("tlsSettings = %#v", stream["tlsSettings"])
	}
}

func TestBuildXrayOutboundRealityStreamUsesModernCredentialField(t *testing.T) {
	stream := buildXrayOutboundStream(Outbound{
		Address:          "proxy.example.com",
		Transport:        "raw",
		TLSMode:          "reality",
		ServerName:       "www.example.com",
		Fingerprint:      "chrome",
		RealityPublicKey: "public-key-value",
		RealityShortID:   "0123456789abcdef",
	})
	reality, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		t.Fatalf("realitySettings = %#v", stream["realitySettings"])
	}
	if reality["password"] != "public-key-value" {
		t.Fatalf("REALITY password = %#v", reality["password"])
	}
	if reality["shortId"] != "0123456789abcdef" {
		t.Fatalf("REALITY shortId = %#v", reality["shortId"])
	}
}

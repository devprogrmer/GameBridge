// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeOpenVPNOutboundSpec(t *testing.T) {
	x, err := normalizeOpenVPNOutboundSpec(OpenVPNOutboundSpec{
		Name:         "ovpn-main",
		Interface:    "gbv-test",
		RoutingTable: 12001,
		Mark:         12001,
		Profile:      "client\nremote vpn.example 1194\n<ca>\nCERT\n</ca>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if x.Interface != "gbv-test" || x.Mark != 12001 {
		t.Fatalf("unexpected normalized spec: %+v", x)
	}
}

func TestOpenVPNPathsRejectTraversal(t *testing.T) {
	for _, name := range []string{"../vpn", "vpn/../../evil", ".", "..", "/tmp/vpn"} {
		if _, err := openVPNConfigPath(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	cfg, err := openVPNConfigPath("ovpn-main")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(managedOpenVPNOutboundDir, "ovpn-main.ovpn")
	if cfg != want {
		t.Fatalf("unexpected config path %q; want %q", cfg, want)
	}
}

func TestSanitizeOpenVPNProfileEnforcesManagedRouting(t *testing.T) {
	profile := "client\nremote vpn.example 1194\n<ca>\nCERT\n</ca>\n"
	got, err := sanitizeOpenVPNProfile(profile, "gbv-test", "/etc/gamebridge/openvpn-outbounds/ovpn.auth", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"dev gbv-test",
		"dev-type tun",
		"route-nopull",
		`pull-filter ignore "redirect-gateway"`,
		"auth-user-pass /etc/gamebridge/openvpn-outbounds/ovpn.auth",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("managed profile missing %q:\n%s", want, got)
		}
	}
}

func TestSanitizeOpenVPNProfileRejectsHostRoutingAndScripts(t *testing.T) {
	bad := []string{
		"client\nredirect-gateway def1\nremote vpn.example 1194",
		"client\nroute 0.0.0.0 0.0.0.0\nremote vpn.example 1194",
		"client\nscript-security 2\nup /tmp/evil.sh\nremote vpn.example 1194",
		"client\nplugin /tmp/evil.so\nremote vpn.example 1194",
		"client\ndev tap0\nremote vpn.example 1194",
	}
	for _, profile := range bad {
		if _, err := sanitizeOpenVPNProfile(profile, "gbv-test", "/tmp/auth", false); err == nil {
			t.Fatalf("expected profile to be rejected:\n%s", profile)
		}
	}
}

func TestRenderOpenVPNRouteScriptUsesDedicatedTableAndMark(t *testing.T) {
	got, err := renderOpenVPNRouteScript("gbv-test", 12345, 23456)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"IFACE=gbv-test",
		"TABLE=12345",
		"MARK=23456",
		`ip route replace default dev "$IFACE" table "$TABLE"`,
		`ip rule add fwmark "$MARK" table "$TABLE"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("route script missing %q:\n%s", want, got)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeWGOutboundSpec(t *testing.T) {
	x, err := normalizeWGOutboundSpec(WGOutboundSpec{
		Interface:     "gbw-test",
		Address:       "10.50.0.2/32",
		PrivateKey:    "private",
		PeerPublicKey: "public",
		Endpoint:      "203.0.113.10:51820",
	})
	if err != nil {
		t.Fatal(err)
	}
	if x.MTU != 1420 || x.PersistentKeepalive != 25 {
		t.Fatalf("unexpected defaults: %+v", x)
	}
	if len(x.AllowedIPs) != 2 {
		t.Fatalf("unexpected allowed IPs: %#v", x.AllowedIPs)
	}
}

func TestRenderWGOutboundConfigUsesTableOff(t *testing.T) {
	cfg := renderWGOutboundConfig(WGOutboundSpec{
		Interface:           "gbw-test",
		Address:             "10.50.0.2/32",
		PrivateKey:          "private",
		PeerPublicKey:       "public",
		Endpoint:            "203.0.113.10:51820",
		AllowedIPs:          []string{"0.0.0.0/0", "::/0"},
		PersistentKeepalive: 25,
		MTU:                 1420,
	})
	for _, want := range []string{"Table = off", "AllowedIPs = 0.0.0.0/0, ::/0", "PersistentKeepalive = 25"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("config missing %q:\n%s", want, cfg)
		}
	}
}

func TestNormalizeWGOutboundSpecRejectsLongInterface(t *testing.T) {
	_, err := normalizeWGOutboundSpec(WGOutboundSpec{
		Interface:     "this-interface-is-too-long",
		Address:       "10.50.0.2/32",
		PrivateKey:    "private",
		PeerPublicKey: "public",
		Endpoint:      "203.0.113.10:51820",
	})
	if err == nil {
		t.Fatal("expected interface validation error")
	}
}
func TestSafeWGPathComponentRejectsTraversal(t *testing.T) {
	bad := []string{"../wg0", "wg0/../../evil", ".", "..", "/tmp/wg0"}
	for _, iface := range bad {
		if _, err := safeWGPathComponent(iface); err == nil {
			t.Fatalf("expected interface %q to be rejected", iface)
		}
	}
}

func TestWireGuardPathHelpersUseSafeBasename(t *testing.T) {
	cfg, err := wgConfigPath("gbw-test")
	if err != nil {
		t.Fatal(err)
	}
	wantCfg := filepath.Join("/etc/wireguard", "gbw-test.conf")
	if cfg != wantCfg {
		t.Fatalf("unexpected config path %q; want %q", cfg, wantCfg)
	}

	marker, err := managedWGMarker("gbw-test")
	if err != nil {
		t.Fatal(err)
	}
	wantMarker := filepath.Join(managedWGOutboundDir, "gbw-test.json")
	if marker != wantMarker {
		t.Fatalf("unexpected marker path %q; want %q", marker, wantMarker)
	}
}

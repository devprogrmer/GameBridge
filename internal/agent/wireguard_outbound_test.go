// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
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

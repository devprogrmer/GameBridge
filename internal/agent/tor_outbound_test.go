// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTorOutboundSpec(t *testing.T) {
	x, err := normalizeTorOutboundSpec(TorOutboundSpec{Name: "tor-main", SocksPort: 19050})
	if err != nil {
		t.Fatal(err)
	}
	if x.Name != "tor-main" || x.SocksPort != 19050 {
		t.Fatalf("unexpected normalized Tor spec: %+v", x)
	}
}

func TestNormalizeTorOutboundSpecRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../tor", "tor/../../evil", ".", "..", "/tmp/tor"} {
		if _, err := normalizeTorOutboundSpec(TorOutboundSpec{Name: name, SocksPort: 19050}); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestTorManagedPaths(t *testing.T) {
	cfg, err := torConfigPath("tor-main")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(managedTorOutboundDir, "tor-main.torrc")
	if cfg != want {
		t.Fatalf("unexpected config path %q; want %q", cfg, want)
	}
}

func TestRenderTorConfigLocalOnly(t *testing.T) {
	cfg := renderTorConfig(TorOutboundSpec{Name: "tor-main", SocksPort: 19050}, "/var/lib/gamebridge/tor-outbounds/tor-main")
	for _, want := range []string{
		"SocksPort 127.0.0.1:19050",
		"IsolateSOCKSAuth",
		"ClientOnly 1",
		"AvoidDiskWrites 1",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("Tor config missing %q:\n%s", want, cfg)
		}
	}
}

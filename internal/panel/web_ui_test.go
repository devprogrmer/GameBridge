// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"strings"
	"testing"
)

func TestControlCenterIsCSPFriendly(t *testing.T) {
	for _, name := range []string{"web/index.html", "web/app.js"} {
		b, err := webFS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lower := strings.ToLower(string(b))
		for _, forbidden := range []string{
			"onclick=",
			"onchange=",
			"onsubmit=",
			"onload=",
			"javascript:",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains CSP-incompatible inline handler %q", name, forbidden)
			}
		}
	}
}

func TestControlCenterUsesUnifiedFrontend(t *testing.T) {
	b, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(b)
	if strings.Contains(index, "phase2.js") || strings.Contains(index, "phase3.js") {
		t.Fatal("legacy phase scripts are still referenced by index.html")
	}
	if !strings.Contains(index, `src="/app.js"`) {
		t.Fatal("unified app.js is not referenced")
	}
}

func TestControlCenterCoversCoreNetworkAPIs(t *testing.T) {
	b, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(b)
	required := []string{
		"api('inbounds')",
		"api('outbounds')",
		"api('outbound-groups')",
		"api('routing')",
		"`outbounds/${id}/probe`",
		"`outbounds/${id}/health`",
		"`outbound-groups/${id}/summary`",
	}
	for _, needle := range required {
		if !strings.Contains(app, needle) {
			t.Fatalf("control center missing core API integration %q", needle)
		}
	}
}

func TestProfessionalUXBranding(t *testing.T) {
	indexBytes, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	styleBytes, err := webFS.ReadFile("web/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexBytes)
	styles := string(styleBytes)

	for _, needle := range []string{
		`class="gb-logo large"`,
		`class="gb-logo small"`,
		`id="global-search"`,
		`class="env-chip"`,
	} {
		if !strings.Contains(index, needle) {
			t.Fatalf("professional shell missing %q", needle)
		}
	}
	for _, needle := range []string{
		".page-hero",
		".resource-grid",
		".failover-flow",
		".route-card",
		".audit-timeline",
		".gb-logo",
	} {
		if !strings.Contains(styles, needle) {
			t.Fatalf("professional stylesheet missing %q", needle)
		}
	}
}

func TestProfessionalUXHasDedicatedOperationalPages(t *testing.T) {
	b, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	app := string(b)
	required := []string{
		"Node Fleet",
		"Inbound Gateway",
		"Outbound Control Center",
		"Failover & Load Balancing",
		"Routing Rules",
		"Traffic Analytics",
		"GameBridge Tunnels",
		"Audit Timeline",
		"openOutboundEdit",
		"openGroupEdit",
		"openRoutingEdit",
		"openUserEdit",
		"openPlanEdit",
		"openInboundEdit",
		"probeAllOutbounds",
		"applyPageFilter",
	}
	for _, needle := range required {
		if !strings.Contains(app, needle) {
			t.Fatalf("professional UX missing %q", needle)
		}
	}
}

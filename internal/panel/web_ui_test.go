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
			"style=",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s contains CSP-incompatible inline content %q", name, forbidden)
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
		`class="brand-logo large"`,
		`class="brand-logo small"`,
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
		".brand-logo",
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

func TestFinalPolishHasFirstPartyBrandAsset(t *testing.T) {
	indexBytes, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	brandBytes, err := webFS.ReadFile("web/brand.svg")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexBytes)
	brand := strings.ToLower(string(brandBytes))

	if !strings.Contains(index, `rel="icon" type="image/svg+xml" href="/brand.svg"`) {
		t.Fatal("GameBridge brand asset is not configured as favicon")
	}
	if !strings.Contains(index, `src="/brand.svg" alt="GameBridge"`) {
		t.Fatal("GameBridge brand asset is not used by the application shell")
	}
	if !strings.Contains(brand, "<svg") || strings.Contains(brand, "<script") {
		t.Fatal("brand.svg must be a script-free SVG")
	}
}

func TestFinalPolishHasLiveTelemetryChartsAndSafeConfirmation(t *testing.T) {
	appBytes, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	styleBytes, err := webFS.ReadFile("web/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	indexBytes, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}

	app := string(appBytes)
	styles := string(styleBytes)
	index := string(indexBytes)

	for _, needle := range []string{
		"donutChart",
		"trafficChart",
		"fleetChart",
		"progressMeter",
		"startLiveRefresh",
		"toggleLiveRefresh",
		"confirmAction",
		"livePages",
		"NODE TELEMETRY",
		"OUTBOUND TELEMETRY",
		"USER TELEMETRY",
	} {
		if !strings.Contains(app, needle) {
			t.Fatalf("final product polish missing %q", needle)
		}
	}
	if strings.Contains(app, "window.confirm(") {
		t.Fatal("native browser confirm is still used")
	}
	if !strings.Contains(index, `id="live-toggle"`) {
		t.Fatal("live telemetry control is missing from the topbar")
	}
	for _, needle := range []string{
		".donut-chart",
		".traffic-chart",
		".live-toggle",
		".confirm-panel",
		".usage-progress",
		".node-detail-grid",
	} {
		if !strings.Contains(styles, needle) {
			t.Fatalf("final polish stylesheet missing %q", needle)
		}
	}
}

// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"strings"
	"testing"
)

func TestUUIDV4(t *testing.T) {
	v := uuidV4()
	if len(v) != 36 || strings.Count(v, "-") != 4 {
		t.Fatalf("bad uuid: %q", v)
	}
	if v[14] != '4' {
		t.Fatalf("not v4 uuid: %q", v)
	}
}

func TestInboundValidationReality(t *testing.T) {
	in := inboundCreate{
		Name:               "reality",
		Protocol:           "vless",
		NodeID:             "n1",
		Port:               443,
		Transport:          "tcp",
		TLSMode:            "reality",
		RealityDest:        "www.cloudflare.com:443",
		RealityServerNames: []string{"www.cloudflare.com"},
	}
	if err := normalizeInboundInput(&in); err != nil {
		t.Fatal(err)
	}
	if len(in.RealityShortIDs) != 1 {
		t.Fatalf("short id was not generated")
	}
}

func TestCurrentXrayTransportMapping(t *testing.T) {
	// This test documents the panel-facing names; buildXrayConfig maps them
	// to current Xray streamSettings.method values (tcp->raw, ws->websocket).
	for _, v := range []string{"tcp", "ws", "grpc"} {
		if !validTransport(v) {
			t.Fatalf("transport %q should be valid", v)
		}
	}
}

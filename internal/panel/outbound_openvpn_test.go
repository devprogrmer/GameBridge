// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNormalizeOpenVPNOutboundInput(t *testing.T) {
	in := outboundInput{
		Name:     "OpenVPN Main",
		NodeID:   "node-1",
		Tag:      "ovpn-main",
		Protocol: "openvpn",
		Secret:   "client\nremote vpn.example 1194",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.OpenVPNInterface == "" {
		t.Fatal("expected default openvpn interface")
	}
	if in.OpenVPNRoutingTable < 1000 || in.OpenVPNMark != in.OpenVPNRoutingTable {
		t.Fatalf("unexpected openvpn routing defaults: table=%d mark=%d", in.OpenVPNRoutingTable, in.OpenVPNMark)
	}
	if !outboundProtocolNeedsSecret("openvpn") {
		t.Fatal("openvpn outbound must require an encrypted profile")
	}
}

func TestDefaultOpenVPNRoutingIDStable(t *testing.T) {
	a := defaultOpenVPNRoutingID("ovpn-main")
	b := defaultOpenVPNRoutingID("ovpn-main")
	if a != b || a < 10000 || a > 59999 {
		t.Fatalf("unexpected deterministic routing id: %d %d", a, b)
	}
}

func TestOpenVPNCredentialRoundTrip(t *testing.T) {
	raw, err := encodeOpenVPNCredential("client\nremote vpn.example 1194", "secret-pass")
	if err != nil {
		t.Fatal(err)
	}
	cred, err := decodeOpenVPNCredential(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Profile == "" || cred.Password != "secret-pass" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

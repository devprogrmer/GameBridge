// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNormalizeWireGuardOutboundDefaults(t *testing.T) {
	in := outboundInput{
		Name:                   "wg-exit",
		NodeID:                 "node-1",
		Tag:                    "wg-exit",
		Protocol:               "wireguard",
		Address:                "203.0.113.10",
		Port:                   51820,
		Secret:                 "client-private-key",
		WireGuardAddress:       "10.50.0.2/32",
		WireGuardPeerPublicKey: "server-public-key",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.WireGuardInterface == "" || len(in.WireGuardInterface) > 15 {
		t.Fatalf("unexpected interface %q", in.WireGuardInterface)
	}
	if in.WireGuardMTU != 1420 {
		t.Fatalf("expected MTU 1420, got %d", in.WireGuardMTU)
	}
	if in.WireGuardKeepalive != 25 {
		t.Fatalf("expected keepalive 25, got %d", in.WireGuardKeepalive)
	}
	if len(in.WireGuardAllowedIPs) != 2 {
		t.Fatalf("expected dual-stack allowed IP defaults, got %#v", in.WireGuardAllowedIPs)
	}
}

func TestNormalizeWireGuardRejectsBadCIDR(t *testing.T) {
	in := outboundInput{
		Name:                   "wg",
		NodeID:                 "node-1",
		Tag:                    "wg",
		Protocol:               "wireguard",
		Address:                "203.0.113.10",
		Port:                   51820,
		Secret:                 "private",
		WireGuardAddress:       "not-a-cidr",
		WireGuardPeerPublicKey: "public",
	}
	if err := normalizeOutboundInput(&in); err == nil {
		t.Fatal("expected invalid CIDR error")
	}
}

func TestWireGuardOutboundRequiresSecret(t *testing.T) {
	if !outboundProtocolNeedsSecret("wireguard") {
		t.Fatal("wireguard outbound must require an encrypted private key")
	}
}

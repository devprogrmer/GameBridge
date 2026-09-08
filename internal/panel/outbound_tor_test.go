// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNormalizeTorOutboundInput(t *testing.T) {
	in := outboundInput{
		Name:     "Tor Main",
		NodeID:   "node-1",
		Tag:      "tor-main",
		Protocol: "tor",
	}
	if err := normalizeOutboundInput(&in); err != nil {
		t.Fatal(err)
	}
	if in.TorSOCKSPort != 19050 {
		t.Fatalf("expected default Tor SOCKS port 19050, got %d", in.TorSOCKSPort)
	}
	if outboundProtocolNeedsSecret("tor") {
		t.Fatal("Tor outbound must not require a credential")
	}
}

func TestNormalizeTorOutboundInputRejectsPrivilegedPort(t *testing.T) {
	in := outboundInput{
		Name:         "Tor Main",
		NodeID:       "node-1",
		Tag:          "tor-main",
		Protocol:     "tor",
		TorSOCKSPort: 905,
	}
	if err := normalizeOutboundInput(&in); err == nil {
		t.Fatal("expected invalid Tor SOCKS port error")
	}
}

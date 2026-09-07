// SPDX-License-Identifier: AGPL-3.0-only
package agent

import "testing"

func TestParseX25519Output(t *testing.T) {
	priv, pub := parseX25519Output("Private key: private-value\nPublic key: public-value\n")
	if priv != "private-value" || pub != "public-value" {
		t.Fatalf("unexpected keys: %q %q", priv, pub)
	}

	priv, pub = parseX25519Output("PrivateKey: p2\nPassword: q2\n")
	if priv != "p2" || pub != "q2" {
		t.Fatalf("unexpected new-format keys: %q %q", priv, pub)
	}
}

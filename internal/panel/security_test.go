// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"testing"
	"time"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, e := hashPassword("a-very-strong-password")
	if e != nil {
		t.Fatal(e)
	}
	if !verifyPassword(h, "a-very-strong-password") {
		t.Fatal("verify failed")
	}
	if verifyPassword(h, "wrong-password") {
		t.Fatal("wrong password verified")
	}
}
func TestSessionRoundTrip(t *testing.T) {
	k := make([]byte, 32)
	p := sessionPayload{AdminID: "1", Username: "owner", Role: "owner", Exp: time.Now().Add(time.Hour).Unix()}
	tok, e := signSession(k, p)
	if e != nil {
		t.Fatal(e)
	}
	got, ok := verifySession(k, tok)
	if !ok || got.AdminID != "1" {
		t.Fatal("session failed")
	}
}
func TestDerivePair(t *testing.T) {
	a, b, ai, bi, e := derivePair("10.20.0.0/30")
	if e != nil {
		t.Fatal(e)
	}
	if a != "10.20.0.1/30" || b != "10.20.0.2/30" || ai != "10.20.0.1" || bi != "10.20.0.2" {
		t.Fatalf("%s %s %s %s", a, b, ai, bi)
	}
}

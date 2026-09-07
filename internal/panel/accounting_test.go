// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "testing"

func TestNextWGAddressStartsAfterServerAndAvoidsUsed(t *testing.T) {
	addr, err := nextWGAddress("10.77.0.1/24", map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if addr != "10.77.0.2/32" {
		t.Fatalf("first client should be .2, got %s", addr)
	}

	addr, err = nextWGAddress("10.77.0.1/24", map[string]bool{"10.77.0.2": true, "10.77.0.3": true})
	if err != nil {
		t.Fatal(err)
	}
	if addr != "10.77.0.4/32" {
		t.Fatalf("used addresses were not skipped, got %s", addr)
	}
}

func TestEffectiveResetInterval(t *testing.T) {
	st := State{Plans: []Plan{{ID: "p", ResetIntervalDays: 30}}}
	u := User{PlanID: "p"}
	if got := effectiveResetIntervalDays(&st, u); got != 30 {
		t.Fatalf("expected plan reset 30, got %d", got)
	}
	u.ResetIntervalDays = 7
	if got := effectiveResetIntervalDays(&st, u); got != 7 {
		t.Fatalf("user override should win, got %d", got)
	}
}

func TestValidateWGClientAddress(t *testing.T) {
	used := map[string]bool{"10.77.0.2": true}
	if _, err := validateWGClientAddress("10.77.0.1/24", "10.77.0.2/32", used); err == nil {
		t.Fatal("expected duplicate client address to fail")
	}
	got, err := validateWGClientAddress("10.77.0.1/24", "10.77.0.10/32", used)
	if err != nil {
		t.Fatal(err)
	}
	if got != "10.77.0.10/32" {
		t.Fatalf("unexpected normalized address: %s", got)
	}
}

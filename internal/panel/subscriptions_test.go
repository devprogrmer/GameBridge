// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"testing"
	"time"
)

func TestUserLifecycleStatus(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	st := State{
		Plans: []Plan{{ID: "p", DataLimitBytes: 100}},
	}

	tests := []struct {
		name string
		user User
		want string
	}{
		{"active", User{Status: UserStatusActive, PlanID: "p", TrafficUsedBytes: 50}, UserStatusActive},
		{"disabled", User{Status: UserStatusDisabled, PlanID: "p"}, UserStatusDisabled},
		{"expired", User{Status: UserStatusActive, PlanID: "p", ExpiresAt: now.Add(-time.Second)}, UserStatusExpired},
		{"quota", User{Status: UserStatusActive, PlanID: "p", TrafficUsedBytes: 100}, UserStatusQuotaExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := userLifecycleStatus(&st, tt.user, now)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReconcileTrafficReset(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	st := State{
		Plans: []Plan{{
			ID:                "p",
			DataLimitBytes:    100,
			ResetIntervalDays: 30,
			DeviceLimit:       2,
			Enabled:           true,
		}},
		Users: []User{{
			ID:                    "u",
			Status:                UserStatusQuotaExceeded,
			PlanID:                "p",
			TrafficUsedBytes:      100,
			XrayTrafficBytes:      60,
			WireGuardTrafficBytes: 40,
			NextTrafficResetAt:    now.Add(-time.Minute),
		}},
	}

	reconcileSubscriptions(&st, now)
	u := st.Users[0]
	if u.TrafficUsedBytes != 0 || u.XrayTrafficBytes != 0 || u.WireGuardTrafficBytes != 0 {
		t.Fatalf("traffic counters were not reset: %+v", u)
	}
	if u.Status != UserStatusActive {
		t.Fatalf("expected active after reset, got %q", u.Status)
	}
	if !u.NextTrafficResetAt.After(now) {
		t.Fatalf("next reset was not advanced")
	}
}

func TestUserEntitlements(t *testing.T) {
	now := time.Now().UTC()
	st := State{
		Plans: []Plan{{
			ID:             "p",
			DataLimitBytes: 1000,
			DeviceLimit:    3,
		}},
	}
	u := User{Status: UserStatusActive, PlanID: "p", TrafficUsedBytes: 250}
	ent := userEntitlements(&st, u, now)
	if ent.TrafficLeftBytes != 750 {
		t.Fatalf("traffic left = %d, want 750", ent.TrafficLeftBytes)
	}
	if ent.DeviceLimit != 3 {
		t.Fatalf("device limit = %d, want 3", ent.DeviceLimit)
	}
}

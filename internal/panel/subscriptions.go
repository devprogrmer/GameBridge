// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	UserStatusActive        = "active"
	UserStatusDisabled      = "disabled"
	UserStatusExpired       = "expired"
	UserStatusQuotaExceeded = "quota_exceeded"
)

type UserEntitlements struct {
	DataLimitBytes    int64     `json:"data_limit_bytes"`
	TrafficUsedBytes  int64     `json:"traffic_used_bytes"`
	TrafficLeftBytes  int64     `json:"traffic_left_bytes"`
	UnlimitedTraffic  bool      `json:"unlimited_traffic"`
	DeviceLimit       int       `json:"device_limit"`
	ResetIntervalDays int       `json:"reset_interval_days"`
	ExpiresAt         time.Time `json:"expires_at"`
	NextResetAt       time.Time `json:"next_reset_at"`
	Status            string    `json:"status"`
	StatusReason      string    `json:"status_reason,omitempty"`
}

func normalizeUserStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", UserStatusActive:
		return UserStatusActive
	case UserStatusDisabled:
		return UserStatusDisabled
	case UserStatusExpired:
		return UserStatusExpired
	case UserStatusQuotaExceeded:
		return UserStatusQuotaExceeded
	default:
		return UserStatusActive
	}
}

func userLifecycleStatus(st *State, u User, now time.Time) (string, string) {
	if normalizeUserStatus(u.Status) == UserStatusDisabled {
		return UserStatusDisabled, "manually disabled"
	}
	if !u.ExpiresAt.IsZero() && !now.Before(u.ExpiresAt) {
		return UserStatusExpired, "subscription expired"
	}
	limit := effectiveUserLimit(st, u)
	if limit > 0 && u.TrafficUsedBytes >= limit {
		return UserStatusQuotaExceeded, "traffic quota exceeded"
	}
	return UserStatusActive, ""
}

func userEntitlements(st *State, u User, now time.Time) UserEntitlements {
	status, reason := userLifecycleStatus(st, u, now)
	limit := effectiveUserLimit(st, u)
	left := int64(-1)
	unlimited := limit <= 0
	if !unlimited {
		left = limit - u.TrafficUsedBytes
		if left < 0 {
			left = 0
		}
	}
	return UserEntitlements{
		DataLimitBytes:    limit,
		TrafficUsedBytes:  u.TrafficUsedBytes,
		TrafficLeftBytes:  left,
		UnlimitedTraffic:  unlimited,
		DeviceLimit:       effectiveDeviceLimit(st, u),
		ResetIntervalDays: effectiveResetIntervalDays(st, u),
		ExpiresAt:         u.ExpiresAt,
		NextResetAt:       u.NextTrafficResetAt,
		Status:            status,
		StatusReason:      reason,
	}
}

func advanceResetAt(t time.Time, days int, now time.Time) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	if t.IsZero() {
		return now.Add(time.Duration(days) * 24 * time.Hour)
	}
	for !t.After(now) {
		t = t.Add(time.Duration(days) * 24 * time.Hour)
	}
	return t
}

func reconcileSubscriptions(st *State, now time.Time) []string {
	nodes := map[string]struct{}{}
	for i := range st.Users {
		u := &st.Users[i]
		changed := false

		if days := effectiveResetIntervalDays(st, *u); days > 0 &&
			!u.NextTrafficResetAt.IsZero() &&
			!now.Before(u.NextTrafficResetAt) {
			u.TrafficUsedBytes = 0
			u.XrayTrafficBytes = 0
			u.WireGuardTrafficBytes = 0
			u.LastTrafficResetAt = now
			u.NextTrafficResetAt = advanceResetAt(u.NextTrafficResetAt, days, now)
			changed = true
		}

		status, reason := userLifecycleStatus(st, *u, now)
		if u.Status != status || u.StatusReason != reason {
			u.Status = status
			u.StatusReason = reason
			changed = true
		}
		if changed {
			u.UpdatedAt = now
			for _, b := range st.InboundClients {
				if b.UserID == u.ID {
					if in := findInbound(st, b.InboundID); in != nil {
						nodes[in.NodeID] = struct{}{}
					}
				}
			}
		}
	}

	out := make([]string, 0, len(nodes))
	for id := range nodes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *Server) reconcileSubscriptionsAndDeploy() {
	var nodeIDs []string
	if err := s.store.Update(func(st *State) error {
		nodeIDs = reconcileSubscriptions(st, time.Now().UTC())
		return nil
	}); err != nil {
		return
	}
	for _, id := range nodeIDs {
		_ = s.deployXrayNode(id)
	}
}

func (s *Server) loadUserEntitlements(id string) (User, UserEntitlements, error) {
	var user User
	var ent UserEntitlements
	err := s.store.Read(func(st State) error {
		u := findUser(&st, id)
		if u == nil {
			return errors.New("user not found")
		}
		user = *u
		ent = userEntitlements(&st, *u, time.Now().UTC())
		return nil
	})
	return user, ent, err
}

func (s *Server) handleUserSubscriptionAction(w http.ResponseWriter, r *http.Request, id, action string) {
	if requireRole(r, "admin") != nil {
		jsonError(w, http.StatusForbidden, "forbidden")
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now().UTC()
	var nodeIDs []string
	var auditAction = action

	err := s.store.Update(func(st *State) error {
		u := findUser(st, id)
		if u == nil {
			return errors.New("user not found")
		}

		switch action {
		case "enable":
			u.Status = UserStatusActive
			u.StatusReason = ""
		case "disable":
			u.Status = UserStatusDisabled
			u.StatusReason = "manually disabled"
		case "reset-traffic":
			u.TrafficUsedBytes = 0
			u.XrayTrafficBytes = 0
			u.WireGuardTrafficBytes = 0
			u.LastTrafficResetAt = now
			if days := effectiveResetIntervalDays(st, *u); days > 0 {
				u.NextTrafficResetAt = now.Add(time.Duration(days) * 24 * time.Hour)
			} else {
				u.NextTrafficResetAt = time.Time{}
			}
			u.Status, u.StatusReason = userLifecycleStatus(st, *u, now)
		case "renew":
			var in struct {
				Days int `json:"days"`
			}
			if err := decodeJSON(r, &in); err != nil {
				return err
			}
			days := in.Days
			if days <= 0 {
				if p := findPlan(st, u.PlanID); p != nil {
					days = p.DurationDays
				}
			}
			if days <= 0 {
				return errors.New("renewal days must be greater than zero")
			}
			base := now
			if u.ExpiresAt.After(now) {
				base = u.ExpiresAt
			}
			u.ExpiresAt = base.Add(time.Duration(days) * 24 * time.Hour)
			if u.Status != UserStatusDisabled {
				u.Status = UserStatusActive
				u.StatusReason = ""
			}
		case "regenerate-token":
			u.SubscriptionToken = randomHex(24)
		case "apply-plan":
			var in struct {
				PlanID string `json:"plan_id"`
				Reset  bool   `json:"reset_traffic"`
			}
			if err := decodeJSON(r, &in); err != nil {
				return err
			}
			p := findPlan(st, in.PlanID)
			if p == nil {
				return errors.New("plan not found")
			}
			if !p.Enabled {
				return errors.New("plan is disabled")
			}
			u.PlanID = p.ID
			u.DataLimitBytes = 0
			u.DeviceLimit = 0
			u.ResetIntervalDays = 0
			if p.DurationDays > 0 {
				u.ExpiresAt = now.Add(time.Duration(p.DurationDays) * 24 * time.Hour)
			}
			if p.ResetIntervalDays > 0 {
				u.NextTrafficResetAt = now.Add(time.Duration(p.ResetIntervalDays) * 24 * time.Hour)
			} else {
				u.NextTrafficResetAt = time.Time{}
			}
			if in.Reset {
				u.TrafficUsedBytes = 0
				u.XrayTrafficBytes = 0
				u.WireGuardTrafficBytes = 0
				u.LastTrafficResetAt = now
			}
			if u.Status != UserStatusDisabled {
				u.Status = UserStatusActive
				u.StatusReason = ""
			}
		default:
			return errors.New("invalid user action")
		}

		u.UpdatedAt = now
		for _, b := range st.InboundClients {
			if b.UserID == u.ID {
				if in := findInbound(st, b.InboundID); in != nil {
					nodeIDs = append(nodeIDs, in.NodeID)
				}
			}
		}
		return nil
	})
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, nodeID := range nodeIDs {
		_ = s.deployXrayNode(nodeID)
	}
	s.audit(r, auditAction, "user:"+id)

	user, ent, _ := s.loadUserEntitlements(id)
	jsonWrite(w, http.StatusOK, map[string]any{"user": user, "entitlements": ent})
}

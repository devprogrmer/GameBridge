// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu   sync.RWMutex
	path string
	st   State
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	s.st.Schema = 1
	s.st.Settings = map[string]string{"site_name": "GameBridge"}
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &s.st); e != nil {
			return nil, e
		}
		if s.st.Settings == nil {
			s.st.Settings = map[string]string{"site_name": "GameBridge"}
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	return s, nil
}
func (s *Store) Read(fn func(State) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fn(cloneState(s.st))
}
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := fn(&s.st); e != nil {
		return e
	}
	return s.saveLocked()
}
func (s *Store) saveLocked() error {
	if e := os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(s.st, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
func cloneState(st State) State {
	b, _ := json.Marshal(st)
	var o State
	_ = json.Unmarshal(b, &o)
	return o
}
func findAdmin(st *State, id string) *Admin {
	for i := range st.Admins {
		if st.Admins[i].ID == id {
			return &st.Admins[i]
		}
	}
	return nil
}
func findUser(st *State, id string) *User {
	for i := range st.Users {
		if st.Users[i].ID == id {
			return &st.Users[i]
		}
	}
	return nil
}
func findPlan(st *State, id string) *Plan {
	for i := range st.Plans {
		if st.Plans[i].ID == id {
			return &st.Plans[i]
		}
	}
	return nil
}
func findNode(st *State, id string) *Node {
	for i := range st.Nodes {
		if st.Nodes[i].ID == id {
			return &st.Nodes[i]
		}
	}
	return nil
}
func findTunnel(st *State, id string) *Tunnel {
	for i := range st.Tunnels {
		if st.Tunnels[i].ID == id {
			return &st.Tunnels[i]
		}
	}
	return nil
}
func findPeer(st *State, id string) *VPNPeer {
	for i := range st.VPNPeers {
		if st.VPNPeers[i].ID == id {
			return &st.VPNPeers[i]
		}
	}
	return nil
}
func deleteByID[T any](a []T, id string, get func(T) string) []T {
	out := a[:0]
	for _, x := range a {
		if get(x) != id {
			out = append(out, x)
		}
	}
	return out
}
func normalizeUsername(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
func sortAudit(st *State) {
	sort.Slice(st.Audit, func(i, j int) bool { return st.Audit[i].CreatedAt.After(st.Audit[j].CreatedAt) })
	if len(st.Audit) > 2000 {
		st.Audit = st.Audit[:2000]
	}
}
func effectiveUserLimit(st *State, u User) int64 {
	if u.DataLimitBytes > 0 {
		return u.DataLimitBytes
	}
	if p := findPlan(st, u.PlanID); p != nil {
		return p.DataLimitBytes
	}
	return 0
}
func effectiveDeviceLimit(st *State, u User) int {
	if u.DeviceLimit > 0 {
		return u.DeviceLimit
	}
	if p := findPlan(st, u.PlanID); p != nil && p.DeviceLimit > 0 {
		return p.DeviceLimit
	}
	return 1
}
func maybeSetPlanExpiry(st *State, u *User) {
	if !u.ExpiresAt.IsZero() {
		return
	}
	if p := findPlan(st, u.PlanID); p != nil && p.DurationDays > 0 {
		u.ExpiresAt = time.Now().UTC().Add(time.Duration(p.DurationDays) * 24 * time.Hour)
	}
}

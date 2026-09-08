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

type diskAdmin struct {
	Admin
	PasswordHash string `json:"password_hash,omitempty"`
	TOTPSecret   string `json:"totp_secret,omitempty"`
}

type diskNode struct {
	Node
	AgentTokenEnc string `json:"agent_token_enc,omitempty"`
}

type diskInbound struct {
	Inbound
	RealityPrivateKeyEnc         string `json:"reality_private_key_enc,omitempty"`
	ShadowsocksServerPasswordEnc string `json:"shadowsocks_server_password_enc,omitempty"`
}

type diskInboundClient struct {
	InboundClient
	CredentialEnc string `json:"credential_enc,omitempty"`
}

type diskOutbound struct {
	Outbound
	PasswordEnc string `json:"password_enc,omitempty"`
}

type diskVPNPeer struct {
	VPNPeer
	PrivateKeyEnc string `json:"private_key_enc,omitempty"`
	ConfigEnc     string `json:"config_enc,omitempty"`
}

type diskState struct {
	Schema         int                 `json:"schema"`
	Admins         []diskAdmin         `json:"admins"`
	Plans          []Plan              `json:"plans"`
	Users          []User              `json:"users"`
	Nodes          []diskNode          `json:"nodes"`
	Tunnels        []Tunnel            `json:"tunnels"`
	Inbounds       []diskInbound       `json:"inbounds"`
	InboundClients []diskInboundClient `json:"inbound_clients"`
	Outbounds      []diskOutbound      `json:"outbounds"`
	OutboundGroups []OutboundGroup     `json:"outbound_groups"`
	RoutingRules   []RoutingRule       `json:"routing_rules"`
	Forwards       []PortForward       `json:"forwards"`
	VPNPeers       []diskVPNPeer       `json:"vpn_peers"`
	Audit          []AuditEntry        `json:"audit"`
	Settings       map[string]string   `json:"settings"`
}

func stateToDisk(st State) diskState {
	d := diskState{
		Schema:         st.Schema,
		Plans:          append([]Plan(nil), st.Plans...),
		Users:          append([]User(nil), st.Users...),
		Tunnels:        append([]Tunnel(nil), st.Tunnels...),
		OutboundGroups: append([]OutboundGroup(nil), st.OutboundGroups...),
		RoutingRules:   append([]RoutingRule(nil), st.RoutingRules...),
		Forwards:       append([]PortForward(nil), st.Forwards...),
		Audit:          append([]AuditEntry(nil), st.Audit...),
		Settings:       map[string]string{},
	}
	for k, v := range st.Settings {
		d.Settings[k] = v
	}
	for _, x := range st.Admins {
		d.Admins = append(d.Admins, diskAdmin{Admin: x, PasswordHash: x.PasswordHash, TOTPSecret: x.TOTPSecret})
	}
	for _, x := range st.Nodes {
		d.Nodes = append(d.Nodes, diskNode{Node: x, AgentTokenEnc: x.AgentTokenEnc})
	}
	for _, x := range st.Inbounds {
		d.Inbounds = append(d.Inbounds, diskInbound{
			Inbound:                      x,
			RealityPrivateKeyEnc:         x.RealityPrivateKeyEnc,
			ShadowsocksServerPasswordEnc: x.ShadowsocksServerPasswordEnc,
		})
	}
	for _, x := range st.InboundClients {
		d.InboundClients = append(d.InboundClients, diskInboundClient{InboundClient: x, CredentialEnc: x.CredentialEnc})
	}
	for _, x := range st.Outbounds {
		d.Outbounds = append(d.Outbounds, diskOutbound{Outbound: x, PasswordEnc: x.PasswordEnc})
	}
	for _, x := range st.VPNPeers {
		d.VPNPeers = append(d.VPNPeers, diskVPNPeer{VPNPeer: x, PrivateKeyEnc: x.PrivateKeyEnc, ConfigEnc: x.ConfigEnc})
	}
	return d
}

func diskToState(d diskState) State {
	st := State{
		Schema:         d.Schema,
		Plans:          append([]Plan(nil), d.Plans...),
		Users:          append([]User(nil), d.Users...),
		Tunnels:        append([]Tunnel(nil), d.Tunnels...),
		OutboundGroups: append([]OutboundGroup(nil), d.OutboundGroups...),
		RoutingRules:   append([]RoutingRule(nil), d.RoutingRules...),
		Forwards:       append([]PortForward(nil), d.Forwards...),
		Audit:          append([]AuditEntry(nil), d.Audit...),
		Settings:       map[string]string{},
	}
	for k, v := range d.Settings {
		st.Settings[k] = v
	}
	for _, x := range d.Admins {
		v := x.Admin
		v.PasswordHash = x.PasswordHash
		v.TOTPSecret = x.TOTPSecret
		st.Admins = append(st.Admins, v)
	}
	for _, x := range d.Nodes {
		v := x.Node
		v.AgentTokenEnc = x.AgentTokenEnc
		st.Nodes = append(st.Nodes, v)
	}
	for _, x := range d.Inbounds {
		v := x.Inbound
		v.RealityPrivateKeyEnc = x.RealityPrivateKeyEnc
		v.ShadowsocksServerPasswordEnc = x.ShadowsocksServerPasswordEnc
		st.Inbounds = append(st.Inbounds, v)
	}
	for _, x := range d.InboundClients {
		v := x.InboundClient
		v.CredentialEnc = x.CredentialEnc
		st.InboundClients = append(st.InboundClients, v)
	}
	for _, x := range d.Outbounds {
		v := x.Outbound
		v.PasswordEnc = x.PasswordEnc
		st.Outbounds = append(st.Outbounds, v)
	}
	for _, x := range d.VPNPeers {
		v := x.VPNPeer
		v.PrivateKeyEnc = x.PrivateKeyEnc
		v.ConfigEnc = x.ConfigEnc
		st.VPNPeers = append(st.VPNPeers, v)
	}
	return st
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	s.st.Schema = 2
	s.st.Settings = map[string]string{"site_name": "GameBridge"}

	if b, err := os.ReadFile(path); err == nil {
		var d diskState
		if err = json.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		s.st = diskToState(d)
		if s.st.Schema < 2 {
			s.st.Schema = 2
		}
		if s.st.Settings == nil {
			s.st.Settings = map[string]string{"site_name": "GameBridge"}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
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

	next := cloneState(s.st)
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.saveStateLocked(next); err != nil {
		return err
	}
	s.st = next
	return nil
}

func (s *Store) saveStateLocked(st State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(stateToDisk(st), "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func cloneState(st State) State {
	out := st
	out.Admins = append([]Admin(nil), st.Admins...)
	out.Plans = append([]Plan(nil), st.Plans...)
	out.Users = append([]User(nil), st.Users...)
	out.Nodes = append([]Node(nil), st.Nodes...)
	for i := range out.Nodes {
		out.Nodes[i].Tags = append([]string(nil), st.Nodes[i].Tags...)
	}
	out.Tunnels = append([]Tunnel(nil), st.Tunnels...)
	for i := range out.Tunnels {
		out.Tunnels[i].Ports = append([]int(nil), st.Tunnels[i].Ports...)
	}
	out.Inbounds = append([]Inbound(nil), st.Inbounds...)
	for i := range out.Inbounds {
		out.Inbounds[i].RealityServerNames = append([]string(nil), st.Inbounds[i].RealityServerNames...)
		out.Inbounds[i].RealityShortIDs = append([]string(nil), st.Inbounds[i].RealityShortIDs...)
	}
	out.InboundClients = append([]InboundClient(nil), st.InboundClients...)
	out.Outbounds = append([]Outbound(nil), st.Outbounds...)
	for i := range out.Outbounds {
		out.Outbounds[i].WireGuardAllowedIPs = append([]string(nil), st.Outbounds[i].WireGuardAllowedIPs...)
	}
	out.OutboundGroups = append([]OutboundGroup(nil), st.OutboundGroups...)
	for i := range out.OutboundGroups {
		out.OutboundGroups[i].Members = append([]OutboundGroupMember(nil), st.OutboundGroups[i].Members...)
	}
	out.RoutingRules = append([]RoutingRule(nil), st.RoutingRules...)
	for i := range out.RoutingRules {
		out.RoutingRules[i].InboundIDs = append([]string(nil), st.RoutingRules[i].InboundIDs...)
		out.RoutingRules[i].UserIDs = append([]string(nil), st.RoutingRules[i].UserIDs...)
		out.RoutingRules[i].Domains = append([]string(nil), st.RoutingRules[i].Domains...)
		out.RoutingRules[i].IPs = append([]string(nil), st.RoutingRules[i].IPs...)
		out.RoutingRules[i].Protocols = append([]string(nil), st.RoutingRules[i].Protocols...)
	}
	out.Forwards = append([]PortForward(nil), st.Forwards...)
	out.VPNPeers = append([]VPNPeer(nil), st.VPNPeers...)
	out.Audit = append([]AuditEntry(nil), st.Audit...)
	out.Settings = make(map[string]string, len(st.Settings))
	for k, v := range st.Settings {
		out.Settings[k] = v
	}
	return out
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

func findInbound(st *State, id string) *Inbound {
	for i := range st.Inbounds {
		if st.Inbounds[i].ID == id {
			return &st.Inbounds[i]
		}
	}
	return nil
}

func findOutbound(st *State, id string) *Outbound {
	for i := range st.Outbounds {
		if st.Outbounds[i].ID == id {
			return &st.Outbounds[i]
		}
	}
	return nil
}

func findRoutingRule(st *State, id string) *RoutingRule {
	for i := range st.RoutingRules {
		if st.RoutingRules[i].ID == id {
			return &st.RoutingRules[i]
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

func deleteUserVPNPeers(peers []VPNPeer, userID string) []VPNPeer {
	out := peers[:0]
	for _, p := range peers {
		if p.UserID != userID {
			out = append(out, p)
		}
	}
	return out
}

func deleteUserInboundClients(clients []InboundClient, userID string) []InboundClient {
	out := clients[:0]
	for _, c := range clients {
		if c.UserID != userID {
			out = append(out, c)
		}
	}
	return out
}

func deleteString(values []string, target string) []string {
	out := values[:0]
	for _, v := range values {
		if v != target {
			out = append(out, v)
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

func effectiveResetIntervalDays(st *State, u User) int {
	if u.ResetIntervalDays > 0 {
		return u.ResetIntervalDays
	}
	if p := findPlan(st, u.PlanID); p != nil && p.ResetIntervalDays > 0 {
		return p.ResetIntervalDays
	}
	return 0
}

func maybeSetPlanExpiry(st *State, u *User) {
	if u.ExpiresAt.IsZero() {
		if p := findPlan(st, u.PlanID); p != nil && p.DurationDays > 0 {
			u.ExpiresAt = time.Now().UTC().Add(time.Duration(p.DurationDays) * 24 * time.Hour)
		}
	}
	if u.NextTrafficResetAt.IsZero() {
		if days := effectiveResetIntervalDays(st, *u); days > 0 {
			u.NextTrafficResetAt = time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		}
	}
}

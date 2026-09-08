// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	outboundHealthPortMin = 30000
	outboundHealthPortMax = 39999
)

type outboundHealthProbe struct {
	OutboundID  string
	InboundTag  string
	OutboundTag string
	Port        int
}

type outboundProbeRequest struct {
	SocksPort int `json:"socks_port"`
}

type outboundProbeResponse struct {
	Healthy   bool      `json:"healthy"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Target    string    `json:"target,omitempty"`
	Error     string    `json:"error,omitempty"`
}

var outboundHealthBatchMu sync.Mutex

func buildOutboundHealthProbes(st State, nodeID string) ([]outboundHealthProbe, error) {
	usedPorts := map[int]bool{}
	for _, in := range st.Inbounds {
		if in.NodeID == nodeID && in.Enabled {
			usedPorts[in.Port] = true
		}
	}

	var candidates []Outbound
	for _, out := range st.Outbounds {
		if out.NodeID != nodeID || !out.Enabled || out.Protocol == "blackhole" {
			continue
		}
		candidates = append(candidates, out)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ID < candidates[j].ID
	})

	next := outboundHealthPortMin
	probes := make([]outboundHealthProbe, 0, len(candidates))
	for _, out := range candidates {
		for next <= outboundHealthPortMax && usedPorts[next] {
			next++
		}
		if next > outboundHealthPortMax {
			return nil, errors.New("no free local port remains for outbound health probes")
		}
		usedPorts[next] = true
		probes = append(probes, outboundHealthProbe{
			OutboundID:  out.ID,
			InboundTag:  "gb-health-" + out.ID,
			OutboundTag: out.Tag,
			Port:        next,
		})
		next++
	}
	return probes, nil
}

func buildOutboundHealthXray(st State, nodeID string) ([]any, []any, error) {
	probes, err := buildOutboundHealthProbes(st, nodeID)
	if err != nil {
		return nil, nil, err
	}

	inbounds := make([]any, 0, len(probes))
	rules := make([]any, 0, len(probes))
	for _, probe := range probes {
		inbounds = append(inbounds, map[string]any{
			"tag":      probe.InboundTag,
			"listen":   "127.0.0.1",
			"port":     probe.Port,
			"protocol": "socks",
			"settings": map[string]any{
				"auth": "noauth",
				"udp":  false,
			},
		})
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []string{probe.InboundTag},
			"outboundTag": probe.OutboundTag,
			"ruleTag":     "gb-health-route-" + probe.OutboundID,
		})
	}
	return inbounds, rules, nil
}

func outboundHealthProbeFor(st State, nodeID, outboundID string) (outboundHealthProbe, error) {
	probes, err := buildOutboundHealthProbes(st, nodeID)
	if err != nil {
		return outboundHealthProbe{}, err
	}
	for _, probe := range probes {
		if probe.OutboundID == outboundID {
			return probe, nil
		}
	}
	return outboundHealthProbe{}, errors.New("outbound is not health-probeable")
}

func normalizeOutboundHealthStatus(status string) string {
	switch status {
	case "healthy", "unhealthy", "disabled", "not_applicable":
		return status
	default:
		return "unknown"
	}
}

func boundedOutboundHealthError(msg string) string {
	const max = 512
	if len(msg) > max {
		return msg[:max]
	}
	return msg
}

func applyOutboundHealthResult(out *Outbound, result outboundProbeResponse) {
	if out == nil {
		return
	}
	checkedAt := result.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	out.HealthLastCheckedAt = &checkedAt
	out.HealthTarget = result.Target

	if result.Healthy {
		out.HealthStatus = "healthy"
		out.HealthLatencyMS = result.LatencyMS
		out.HealthLastError = ""
		out.HealthFailureCount = 0
		successAt := checkedAt
		out.HealthLastSuccessAt = &successAt
		return
	}

	out.HealthStatus = "unhealthy"
	out.HealthLatencyMS = 0
	out.HealthLastError = boundedOutboundHealthError(result.Error)
	out.HealthFailureCount++
}

func outboundHealthView(out Outbound) map[string]any {
	return map[string]any{
		"outbound_id":     out.ID,
		"node_id":         out.NodeID,
		"tag":             out.Tag,
		"protocol":        out.Protocol,
		"enabled":         out.Enabled,
		"status":          normalizeOutboundHealthStatus(out.HealthStatus),
		"latency_ms":      out.HealthLatencyMS,
		"last_checked_at": out.HealthLastCheckedAt,
		"last_success_at": out.HealthLastSuccessAt,
		"last_error":      out.HealthLastError,
		"failure_count":   out.HealthFailureCount,
		"target":          out.HealthTarget,
	}
}

func (s *Server) probeOutboundHealth(id string) (Outbound, error) {
	var snapshot State
	if err := s.store.Read(func(st State) error {
		snapshot = st
		return nil
	}); err != nil {
		return Outbound{}, err
	}

	out := findOutbound(&snapshot, id)
	if out == nil {
		return Outbound{}, errors.New("outbound not found")
	}
	if !out.Enabled {
		return Outbound{}, errors.New("outbound is disabled")
	}
	if out.Protocol == "blackhole" {
		return Outbound{}, errors.New("blackhole outbound is not health-probeable")
	}

	node := findNode(&snapshot, out.NodeID)
	if node == nil {
		return Outbound{}, errors.New("node not found")
	}
	if !node.Enabled {
		return Outbound{}, errors.New("node is disabled")
	}
	if node.Maintenance {
		return Outbound{}, errors.New("node is in maintenance")
	}

	probe, err := outboundHealthProbeFor(snapshot, out.NodeID, out.ID)
	if err != nil {
		return Outbound{}, err
	}

	var result outboundProbeResponse
	err = s.agentJSON(*node, http.MethodPost, "/v1/xray/outbound-probe", outboundProbeRequest{
		SocksPort: probe.Port,
	}, &result)
	if err != nil {
		result = outboundProbeResponse{
			Healthy:   false,
			CheckedAt: time.Now().UTC(),
			Error:     "agent probe: " + err.Error(),
		}
	}

	var updated Outbound
	if updateErr := s.store.Update(func(st *State) error {
		current := findOutbound(st, id)
		if current == nil {
			return errors.New("outbound disappeared while storing health result")
		}
		applyOutboundHealthResult(current, result)
		updated = *current
		return nil
	}); updateErr != nil {
		return Outbound{}, updateErr
	}
	return updated, nil
}

func (s *Server) probeAllOutboundHealth() {
	if !outboundHealthBatchMu.TryLock() {
		return
	}
	defer outboundHealthBatchMu.Unlock()

	var ids []string
	_ = s.store.Read(func(st State) error {
		for _, out := range st.Outbounds {
			if !out.Enabled || out.Protocol == "blackhole" {
				continue
			}
			node := findNode(&st, out.NodeID)
			if node == nil || !node.Enabled || node.Maintenance {
				continue
			}
			ids = append(ids, out.ID)
		}
		return nil
	})
	sort.Strings(ids)

	const concurrency = 4
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			_, _ = s.probeOutboundHealth(id)
		}()
	}
	wg.Wait()
}

func (s *Server) ensureOutboundHealthRuntime() {
	nodeSet := map[string]bool{}
	_ = s.store.Read(func(st State) error {
		for _, out := range st.Outbounds {
			if !out.Enabled || out.Protocol == "blackhole" {
				continue
			}
			node := findNode(&st, out.NodeID)
			if node == nil || !node.Enabled || node.Maintenance {
				continue
			}
			nodeSet[out.NodeID] = true
		}
		return nil
	})

	var nodeIDs []string
	for id := range nodeSet {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	for _, nodeID := range nodeIDs {
		if err := s.deployOutboundNode(nodeID); err != nil {
			_ = s.store.Update(func(st *State) error {
				for i := range st.Outbounds {
					out := &st.Outbounds[i]
					if out.NodeID == nodeID && out.Enabled && out.Protocol != "blackhole" {
						out.HealthStatus = "unhealthy"
						out.HealthLastError = boundedOutboundHealthError(fmt.Sprintf("health runtime deploy: %v", err))
						out.HealthFailureCount++
						now := time.Now().UTC()
						out.HealthLastCheckedAt = &now
					}
				}
				return nil
			})
		}
	}
}

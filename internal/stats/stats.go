// SPDX-License-Identifier: AGPL-3.0-only
package stats

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Path struct {
	Name     string  `json:"name"`
	RTTMS    float64 `json:"rtt_ms"`
	JitterMS float64 `json:"jitter_ms"`
	Healthy  bool    `json:"healthy"`
	LastSeen string  `json:"last_seen,omitempty"`
}

type Snapshot struct {
	Name       string `json:"name"`
	Transport  string `json:"transport"`
	Role       string `json:"role"`
	TxPackets  uint64 `json:"tx_packets"`
	RxPackets  uint64 `json:"rx_packets"`
	TxBytes    uint64 `json:"tx_bytes"`
	RxBytes    uint64 `json:"rx_bytes"`
	Duplicates uint64 `json:"duplicates"`
	ActivePath string `json:"active_path"`
	Paths      []Path `json:"paths"`
	Updated    string `json:"updated"`
}

type Stats struct {
	Name      string
	Transport string
	Role      string

	TxPackets  atomic.Uint64
	RxPackets  atomic.Uint64
	TxBytes    atomic.Uint64
	RxBytes    atomic.Uint64
	Duplicates atomic.Uint64

	mu         sync.RWMutex
	activePath string
	paths      map[string]Path
}

func New(name, transport, role string) *Stats {
	return &Stats{Name: name, Transport: transport, Role: role, paths: make(map[string]Path)}
}

func (s *Stats) SetActivePath(v string) {
	s.mu.Lock()
	s.activePath = v
	s.mu.Unlock()
}

func (s *Stats) SetPath(p Path) {
	s.mu.Lock()
	s.paths[p.Name] = p
	s.mu.Unlock()
}

func (s *Stats) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Snapshot{
		Name: s.Name, Transport: s.Transport, Role: s.Role,
		TxPackets: s.TxPackets.Load(), RxPackets: s.RxPackets.Load(),
		TxBytes: s.TxBytes.Load(), RxBytes: s.RxBytes.Load(),
		Duplicates: s.Duplicates.Load(), ActivePath: s.activePath,
		Updated: time.Now().Format(time.RFC3339),
	}
	for _, p := range s.paths {
		out.Paths = append(out.Paths, p)
	}
	return out
}

func (s *Stats) WriteLoop(ctx context.Context, path string) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b, _ := json.MarshalIndent(s.Snapshot(), "", "  ")
			_ = os.WriteFile(path, b, 0644)
		}
	}
}

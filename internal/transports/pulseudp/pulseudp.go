// SPDX-License-Identifier: AGPL-3.0-only
package pulseudp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devprogrmer/GameBridge/internal/config"
	"github.com/devprogrmer/GameBridge/internal/secure"
	"github.com/devprogrmer/GameBridge/internal/stats"
)

const (
	kindData = 1
	kindPing = 2
	kindPong = 3
)

type path struct {
	id     int
	port   int
	conn   *net.UDPConn
	remote *net.UDPAddr

	mu       sync.RWMutex
	rtt      time.Duration
	jitter   time.Duration
	lastSeen time.Time
	healthy  bool
}

func (p *path) markSeen() {
	p.mu.Lock()
	p.lastSeen = time.Now()
	p.healthy = true
	p.mu.Unlock()
}

func (p *path) updateRTT(v time.Duration) {
	p.mu.Lock()
	old := p.rtt
	if old == 0 {
		p.rtt = v
		p.jitter = 0
	} else {
		delta := old - v
		if delta < 0 {
			delta = -delta
		}
		p.jitter = time.Duration(float64(p.jitter)*0.75 + float64(delta)*0.25)
		p.rtt = time.Duration(float64(old)*0.75 + float64(v)*0.25)
	}
	p.lastSeen = time.Now()
	p.healthy = true
	p.mu.Unlock()
}

func (p *path) score(timeout time.Duration) (float64, bool, time.Duration, time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.healthy {
		return 0, false, p.rtt, p.jitter
	}
	if !p.lastSeen.IsZero() && time.Since(p.lastSeen) > timeout {
		return 0, false, p.rtt, p.jitter
	}
	r := p.rtt
	if r == 0 {
		r = 999 * time.Millisecond
	}
	// Gaming score: RTT + a stronger jitter penalty.
	score := float64(r) + 2.5*float64(p.jitter)
	return score, true, p.rtt, p.jitter
}

type dedupe struct {
	mu sync.Mutex
	m  map[uint64]time.Time
}

func newDedupe() *dedupe { return &dedupe{m: make(map[uint64]time.Time)} }

func (d *dedupe) seen(seq uint64) bool {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.m[seq]; ok {
		return true
	}
	d.m[seq] = now
	if len(d.m) > 16384 {
		cut := now.Add(-15 * time.Second)
		for k, t := range d.m {
			if t.Before(cut) {
				delete(d.m, k)
			}
		}
	}
	return false
}

func ranked(paths []*path, timeout time.Duration) []*path {
	type item struct {
		p     *path
		score float64
	}
	var x []item
	for _, p := range paths {
		score, ok, _, _ := p.score(timeout)
		if ok {
			x = append(x, item{p: p, score: score})
		}
	}
	if len(x) == 0 {
		for _, p := range paths {
			x = append(x, item{p: p, score: 1e30})
		}
	}
	sort.Slice(x, func(i, j int) bool { return x[i].score < x[j].score })
	out := make([]*path, 0, len(x))
	for _, it := range x {
		out = append(out, it.p)
	}
	return out
}

func Run(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	if c.Role == "client" {
		return runClient(ctx, tun, c, box, st)
	}
	return runServer(ctx, tun, c, box, st)
}

func sendClient(p *path, b []byte) error {
	_, err := p.conn.Write(b)
	return err
}

func sendServer(p *path, b []byte) error {
	p.mu.RLock()
	addr := p.remote
	p.mu.RUnlock()
	if addr == nil {
		return errors.New("path has no peer yet")
	}
	_, err := p.conn.WriteToUDP(b, addr)
	return err
}

func runClient(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	var paths []*path
	for i, port := range c.Ports {
		addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(c.PeerHost, fmt.Sprint(port)))
		if err != nil {
			return err
		}
		conn, err := net.DialUDP("udp", nil, addr)
		if err != nil {
			return err
		}
		paths = append(paths, &path{id: i, port: port, conn: conn, healthy: true, lastSeen: time.Now()})
	}
	return runCommon(ctx, tun, c, box, st, paths, false)
}

func runServer(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	var paths []*path
	for i, port := range c.Ports {
		addr := &net.UDPAddr{IP: net.ParseIP(c.ListenHost), Port: port}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return err
		}
		paths = append(paths, &path{id: i, port: port, conn: conn})
	}
	return runCommon(ctx, tun, c, box, st, paths, true)
}

func runCommon(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats, paths []*path, server bool) error {
	if len(paths) == 0 {
		return errors.New("no UDP paths")
	}
	var seq atomic.Uint64
	dupes := newDedupe()
	timeout := time.Duration(c.PathTimeoutMS) * time.Millisecond

	for _, pp := range paths {
		p := pp
		go func() {
			buf := make([]byte, 65535)
			for {
				var n int
				var addr *net.UDPAddr
				var err error
				if server {
					n, addr, err = p.conn.ReadFromUDP(buf)
				} else {
					n, err = p.conn.Read(buf)
				}
				if err != nil {
					return
				}
				if server && addr != nil {
					p.mu.Lock()
					p.remote = addr
					p.mu.Unlock()
				}
				kind, s, payload, err := box.Open(buf[:n])
				if err != nil {
					continue
				}
				p.markSeen()
				switch kind {
				case kindData:
					if dupes.seen(s) {
						st.Duplicates.Add(1)
						continue
					}
					if _, err := tun.Write(payload); err == nil {
						st.RxPackets.Add(1)
						st.RxBytes.Add(uint64(len(payload)))
					}
				case kindPing:
					frame, err := box.Seal(kindPong, s, payload)
					if err != nil {
						continue
					}
					if server {
						_ = sendServer(p, frame)
					} else {
						_ = sendClient(p, frame)
					}
				case kindPong:
					if len(payload) == 8 {
						ts := int64(binary.BigEndian.Uint64(payload))
						rtt := time.Since(time.Unix(0, ts))
						if rtt > 0 && rtt < 30*time.Second {
							p.updateRTT(rtt)
						}
					}
				}
			}
		}()
	}

	go func() {
		t := time.NewTicker(time.Duration(c.KeepaliveMS) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, p := range paths {
					payload := make([]byte, 8)
					binary.BigEndian.PutUint64(payload, uint64(time.Now().UnixNano()))
					frame, err := box.Seal(kindPing, seq.Add(1), payload)
					if err != nil {
						continue
					}
					if server {
						_ = sendServer(p, frame)
					} else {
						_ = sendClient(p, frame)
					}
				}
			}
		}
	}()

	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			return err
		}
		payload := append([]byte(nil), buf[:n]...)
		s := seq.Add(1)
		frame, err := box.Seal(kindData, s, payload)
		if err != nil {
			continue
		}
		order := ranked(paths, timeout)
		if len(order) == 0 {
			continue
		}
		best := order[0]
		if server {
			_ = sendServer(best, frame)
		} else {
			_ = sendClient(best, frame)
		}
		st.TxPackets.Add(1)
		st.TxBytes.Add(uint64(len(payload)))

		_, _, rtt, jit := best.score(timeout)
		name := fmt.Sprintf("udp/%d", best.port)
		st.SetActivePath(name)
		st.SetPath(stats.Path{Name: name, RTTMS: float64(rtt) / float64(time.Millisecond), JitterMS: float64(jit) / float64(time.Millisecond), Healthy: true, LastSeen: time.Now().Format(time.RFC3339)})

		if c.DuplicateSmall && len(payload) <= c.DuplicateThreshold && len(order) > 1 {
			second := order[1]
			if server {
				_ = sendServer(second, frame)
			} else {
				_ = sendClient(second, frame)
			}
		}
	}
}

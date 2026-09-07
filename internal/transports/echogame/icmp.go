// SPDX-License-Identifier: AGPL-3.0-only
package echogame

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/devprogrmer/GameBridge/internal/config"
	"github.com/devprogrmer/GameBridge/internal/secure"
	"github.com/devprogrmer/GameBridge/internal/stats"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const (
	kindPoll  = 1
	kindReply = 2
)

func Run(ctx context.Context, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	pc, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return err
	}
	defer pc.Close()
	if c.Role == "client" {
		return client(ctx, pc, tun, c, box, st)
	}
	return server(ctx, pc, tun, c, box, st)
}

func client(ctx context.Context, pc *icmp.PacketConn, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	peer := &net.IPAddr{IP: net.ParseIP(c.PeerHost)}
	if peer.IP == nil {
		return fmt.Errorf("invalid peer IP: %s", c.PeerHost)
	}
	outQ := make(chan []byte, 128)
	go tunReader(ctx, tun, outQ, st)

	var seq atomic.Uint32
	go func() {
		buf := make([]byte, 65535)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			m, err := icmp.ParseMessage(1, buf[:n])
			if err != nil || m.Type != ipv4.ICMPTypeEchoReply {
				continue
			}
			echo, ok := m.Body.(*icmp.Echo)
			if !ok || echo.ID != c.ICMPID {
				continue
			}
			kind, _, payload, err := box.Open(echo.Data)
			if err != nil || kind != kindReply || len(payload) == 0 {
				continue
			}
			if _, err := tun.Write(payload); err == nil {
				st.RxPackets.Add(1)
				st.RxBytes.Add(uint64(len(payload)))
			}
		}
	}()

	t := time.NewTicker(time.Duration(c.ICMPPollMS) * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			var payload []byte
			select {
			case payload = <-outQ:
			default:
			}
			s := seq.Add(1)
			frame, err := box.Seal(kindPoll, uint64(s), payload)
			if err != nil {
				continue
			}
			msg := icmp.Message{
				Type: ipv4.ICMPTypeEcho,
				Code: 0,
				Body: &icmp.Echo{ID: c.ICMPID, Seq: int(s & 0xffff), Data: frame},
			}
			wire, _ := msg.Marshal(nil)
			_, _ = pc.WriteTo(wire, peer)
			st.SetActivePath("icmp")
		}
	}
}

func server(ctx context.Context, pc *icmp.PacketConn, tun *os.File, c config.Config, box *secure.Box, st *stats.Stats) error {
	outQ := make(chan []byte, 256)
	go tunReader(ctx, tun, outQ, st)

	buf := make([]byte, 65535)
	for {
		_ = pc.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := pc.ReadFrom(buf)
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}
		if err != nil {
			return err
		}
		m, err := icmp.ParseMessage(1, buf[:n])
		if err != nil || m.Type != ipv4.ICMPTypeEcho {
			continue
		}
		echo, ok := m.Body.(*icmp.Echo)
		if !ok || echo.ID != c.ICMPID {
			continue
		}
		kind, s, payload, err := box.Open(echo.Data)
		if err != nil || kind != kindPoll {
			continue
		}
		if len(payload) > 0 {
			if _, err := tun.Write(payload); err == nil {
				st.RxPackets.Add(1)
				st.RxBytes.Add(uint64(len(payload)))
			}
		}
		var reply []byte
		select {
		case reply = <-outQ:
		default:
		}
		frame, err := box.Seal(kindReply, s, reply)
		if err != nil {
			continue
		}
		resp := icmp.Message{
			Type: ipv4.ICMPTypeEchoReply,
			Code: 0,
			Body: &icmp.Echo{ID: echo.ID, Seq: echo.Seq, Data: frame},
		}
		wire, _ := resp.Marshal(nil)
		_, _ = pc.WriteTo(wire, addr)
		st.SetActivePath("icmp")
	}
}

func tunReader(ctx context.Context, tun *os.File, q chan<- []byte, st *stats.Stats) {
	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			return
		}
		p := append([]byte(nil), buf[:n]...)
		select {
		case q <- p:
			st.TxPackets.Add(1)
			st.TxBytes.Add(uint64(n))
		case <-ctx.Done():
			return
		default:
			// Keep latency bounded. If the queue is full, drop newest instead of buffering indefinitely.
		}
	}
}

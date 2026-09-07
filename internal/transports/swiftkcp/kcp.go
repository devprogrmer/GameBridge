// SPDX-License-Identifier: AGPL-3.0-only
package swiftkcp

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/devprogrmer/GameBridge/internal/config"
	"github.com/devprogrmer/GameBridge/internal/stats"
	kcp "github.com/xtaci/kcp-go/v5"
)

func block(c config.Config) (kcp.BlockCrypt, error) {
	key, err := hex.DecodeString(c.KeyHex)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("KCP key must be 32 bytes")
	}
	return kcp.NewAESBlockCrypt(key)
}

func tune(s *kcp.UDPSession, c config.Config) {
	interval := 10
	resend := 2
	nc := 1
	switch strings.ToLower(c.Profile) {
	case "balanced":
		interval = 15
	case "stable":
		interval = 20
		resend = 2
	case "lossy":
		interval = 20
		resend = 3
	}
	s.SetNoDelay(1, interval, resend, nc)
	s.SetWindowSize(512, 512)
	s.SetACKNoDelay(true)
	s.SetStreamMode(true)
	outerMTU := c.MTU + 80
	if outerMTU > 1400 {
		outerMTU = 1400
	}
	if outerMTU < 900 {
		outerMTU = 900
	}
	_ = s.SetMtu(outerMTU)
}

func Run(ctx context.Context, tun *os.File, c config.Config, st *stats.Stats) error {
	bc, err := block(c)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(c.PeerHost, fmt.Sprint(c.Ports[0]))
	if c.Role == "server" {
		addr = net.JoinHostPort(c.ListenHost, fmt.Sprint(c.Ports[0]))
		ln, err := kcp.ListenWithOptions(addr, bc, c.KCPDataShards, c.KCPParityShards)
		if err != nil {
			return err
		}
		defer ln.Close()
		s, err := ln.AcceptKCP()
		if err != nil {
			return err
		}
		tune(s, c)
		st.SetActivePath("kcp/" + fmt.Sprint(c.Ports[0]))
		return bridge(ctx, tun, s, st)
	}
	s, err := kcp.DialWithOptions(addr, bc, c.KCPDataShards, c.KCPParityShards)
	if err != nil {
		return err
	}
	defer s.Close()
	tune(s, c)
	st.SetActivePath("kcp/" + fmt.Sprint(c.Ports[0]))
	return bridge(ctx, tun, s, st)
}

func bridge(ctx context.Context, tun *os.File, s *kcp.UDPSession, st *stats.Stats) error {
	errCh := make(chan error, 2)

	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := tun.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			if n > 65535 {
				continue
			}
			var hdr [4]byte
			binary.BigEndian.PutUint32(hdr[:], uint32(n))
			if _, err := s.Write(hdr[:]); err != nil {
				errCh <- err
				return
			}
			if _, err := s.Write(buf[:n]); err != nil {
				errCh <- err
				return
			}
			st.TxPackets.Add(1)
			st.TxBytes.Add(uint64(n))
		}
	}()

	go func() {
		for {
			var hdr [4]byte
			if _, err := io.ReadFull(s, hdr[:]); err != nil {
				errCh <- err
				return
			}
			n := int(binary.BigEndian.Uint32(hdr[:]))
			if n <= 0 || n > 65535 {
				errCh <- fmt.Errorf("invalid KCP frame length: %d", n)
				return
			}
			buf := make([]byte, n)
			if _, err := io.ReadFull(s, buf); err != nil {
				errCh <- err
				return
			}
			if _, err := tun.Write(buf); err != nil {
				errCh <- err
				return
			}
			st.RxPackets.Add(1)
			st.RxBytes.Add(uint64(n))
		}
	}()

	select {
	case <-ctx.Done():
		_ = s.SetReadDeadline(time.Now())
		return nil
	case err := <-errCh:
		return err
	}
}

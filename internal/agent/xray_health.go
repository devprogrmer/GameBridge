// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

const (
	outboundHealthPortMin = 30000
	outboundHealthPortMax = 39999
)

var outboundHealthTargets = []string{
	"https://www.gstatic.com/generate_204",
	"https://www.cloudflare.com/cdn-cgi/trace",
}

type xrayOutboundProbeRequest struct {
	SocksPort int `json:"socks_port"`
}

type xrayOutboundProbeResponse struct {
	Healthy   bool      `json:"healthy"`
	LatencyMS int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Target    string    `json:"target,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func validateOutboundHealthPort(port int) error {
	if port < outboundHealthPortMin || port > outboundHealthPortMax {
		return fmt.Errorf("outbound health SOCKS port must be between %d and %d", outboundHealthPortMin, outboundHealthPortMax)
	}
	return nil
}

func boundedProbeError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	const max = 512
	if len(msg) > max {
		msg = msg[:max]
	}
	return msg
}

func outboundHealthHTTPClient(port int) (*http.Client, error) {
	if err := validateOutboundHealthPort(port); err != nil {
		return nil, err
	}

	baseDialer := &net.Dialer{
		Timeout:   4 * time.Second,
		KeepAlive: -1,
	}
	socksDialer, err := xproxy.SOCKS5(
		"tcp",
		net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		nil,
		baseDialer,
	)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if d, ok := socksDialer.(xproxy.ContextDialer); ok {
				return d.DialContext(ctx, network, address)
			}
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, dialErr := socksDialer.Dial(network, address)
				ch <- result{conn: conn, err: dialErr}
			}()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case res := <-ch:
				return res.conn, res.err
			}
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func probeXrayOutbound(port int) xrayOutboundProbeResponse {
	checkedAt := time.Now().UTC()
	client, err := outboundHealthHTTPClient(port)
	if err != nil {
		return xrayOutboundProbeResponse{
			Healthy:   false,
			CheckedAt: checkedAt,
			Error:     boundedProbeError(err),
		}
	}

	var errs []string
	for _, target := range outboundHealthTargets {
		started := time.Now()
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
		if reqErr != nil {
			errs = append(errs, reqErr.Error())
			continue
		}
		req.Header.Set("User-Agent", "GameBridge-Outbound-Health/1")

		resp, doErr := client.Do(req)
		latency := time.Since(started).Milliseconds()
		if doErr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", target, doErr))
			continue
		}

		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 500 {
			return xrayOutboundProbeResponse{
				Healthy:   true,
				LatencyMS: latency,
				CheckedAt: checkedAt,
				Target:    target,
			}
		}
		errs = append(errs, fmt.Sprintf("%s: HTTP %d", target, resp.StatusCode))
	}

	return xrayOutboundProbeResponse{
		Healthy:   false,
		CheckedAt: checkedAt,
		Error:     boundedProbeError(fmt.Errorf("%s", strings.Join(errs, "; "))),
	}
}

func (s *Server) handleXrayOutboundProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var in xrayOutboundProbeRequest
	if err := decodeJSON(r, &in); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateOutboundHealthPort(in.SocksPort); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonWrite(w, http.StatusOK, probeXrayOutbound(in.SocksPort))
}

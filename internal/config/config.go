// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Name      string `json:"name"`
	Role      string `json:"role"`      // client | server
	Transport string `json:"transport"` // pulseudp | pulseudp-mp | kcp | quic | icmp

	ListenHost string `json:"listen_host"`
	PeerHost   string `json:"peer_host"`
	Ports      []int  `json:"ports"`

	Interface string `json:"interface"`
	LocalCIDR string `json:"local_cidr"`
	PeerIP    string `json:"peer_ip"`
	MTU       int    `json:"mtu"`

	KeyHex  string `json:"key_hex"`
	Profile string `json:"profile"` // competitive | balanced | stable | lossy

	DuplicateSmall     bool `json:"duplicate_small_packets"`
	DuplicateThreshold int  `json:"duplicate_threshold"`

	KCPDataShards   int `json:"kcp_data_shards"`
	KCPParityShards int `json:"kcp_parity_shards"`

	ICMPID     int `json:"icmp_id"`
	ICMPPollMS int `json:"icmp_poll_ms"`
	ICMPBurst  int `json:"icmp_burst"`

	NAT               bool   `json:"nat"`
	InternetInterface string `json:"internet_interface"`

	KeepaliveMS   int `json:"keepalive_ms"`
	PathTimeoutMS int `json:"path_timeout_ms"`
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	c.Defaults()
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func (c *Config) Defaults() {
	if c.Name == "" {
		c.Name = "default"
	}
	if c.ListenHost == "" {
		c.ListenHost = "0.0.0.0"
	}
	if c.Interface == "" {
		c.Interface = "gb0"
	}
	if c.MTU == 0 {
		switch strings.ToLower(c.Transport) {
		case "icmp":
			c.MTU = 900
		case "quic":
			c.MTU = 1280
		default:
			c.MTU = 1360
		}
	}
	if c.Profile == "" {
		c.Profile = "competitive"
	}
	if c.DuplicateThreshold == 0 {
		c.DuplicateThreshold = 512
	}
	if c.KeepaliveMS == 0 {
		c.KeepaliveMS = 1000
	}
	if c.PathTimeoutMS == 0 {
		c.PathTimeoutMS = 5000
	}
	if c.ICMPPollMS == 0 {
		c.ICMPPollMS = 8
	}
	if c.ICMPBurst == 0 {
		c.ICMPBurst = 4
	}
	if c.KCPDataShards == 0 && strings.Contains(c.Transport, "kcp") {
		c.KCPDataShards = 10
	}
	if c.KCPParityShards == 0 && strings.Contains(c.Transport, "kcp") {
		switch strings.ToLower(c.Profile) {
		case "lossy":
			c.KCPParityShards = 3
		case "stable":
			c.KCPParityShards = 2
		default:
			c.KCPParityShards = 0
		}
	}
	if strings.EqualFold(c.Profile, "stable") || strings.EqualFold(c.Profile, "lossy") {
		c.DuplicateSmall = true
	}
}

func (c Config) Validate() error {
	role := strings.ToLower(c.Role)
	if role != "client" && role != "server" {
		return errors.New(`role must be "client" or "server"`)
	}
	switch strings.ToLower(c.Transport) {
	case "pulseudp", "pulseudp-mp", "kcp", "quic", "icmp":
	default:
		return fmt.Errorf("unsupported userspace transport: %q", c.Transport)
	}
	if c.LocalCIDR == "" || c.PeerIP == "" {
		return errors.New("local_cidr and peer_ip are required")
	}
	if c.KeyHex == "" {
		return errors.New("key_hex is required")
	}
	if role == "client" && c.PeerHost == "" {
		return errors.New("peer_host is required in client mode")
	}
	if strings.ToLower(c.Transport) != "icmp" && len(c.Ports) == 0 {
		return errors.New("at least one port is required")
	}
	return nil
}

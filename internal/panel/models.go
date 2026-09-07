// SPDX-License-Identifier: AGPL-3.0-only
package panel

import "time"

type Admin struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	TOTPSecret   string    `json:"-"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

type Plan struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	DataLimitBytes    int64     `json:"data_limit_bytes"`
	DurationDays      int       `json:"duration_days"`
	DeviceLimit       int       `json:"device_limit"`
	ResetIntervalDays int       `json:"reset_interval_days"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
}

type User struct {
	ID                    string    `json:"id"`
	Username              string    `json:"username"`
	DisplayName           string    `json:"display_name"`
	Email                 string    `json:"email"`
	Status                string    `json:"status"`
	PlanID                string    `json:"plan_id"`
	ExpiresAt             time.Time `json:"expires_at"`
	DataLimitBytes        int64     `json:"data_limit_bytes"`
	TrafficUsedBytes      int64     `json:"traffic_used_bytes"`
	XrayTrafficBytes      int64     `json:"xray_traffic_bytes"`
	WireGuardTrafficBytes int64     `json:"wireguard_traffic_bytes"`
	DeviceLimit           int       `json:"device_limit"`
	ResetIntervalDays     int       `json:"reset_interval_days"`
	NextTrafficResetAt    time.Time `json:"next_traffic_reset_at"`
	Online                bool      `json:"online"`
	OnlineIPs             int       `json:"online_ips"`
	LastOnlineAt          time.Time `json:"last_online_at"`
	SubscriptionToken     string    `json:"subscription_token"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type NodeMetrics struct {
	Hostname        string  `json:"hostname"`
	Kernel          string  `json:"kernel"`
	CoreVersion     string  `json:"core_version"`
	UptimeSeconds   float64 `json:"uptime_seconds"`
	Load1           float64 `json:"load_1"`
	Load5           float64 `json:"load_5"`
	Load15          float64 `json:"load_15"`
	MemoryTotal     uint64  `json:"memory_total"`
	MemoryAvailable uint64  `json:"memory_available"`
	DiskTotal       uint64  `json:"disk_total"`
	DiskFree        uint64  `json:"disk_free"`
	NetworkRX       uint64  `json:"network_rx"`
	NetworkTX       uint64  `json:"network_tx"`
}

type Node struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Role              string      `json:"role"`
	PublicIP          string      `json:"public_ip"`
	AgentURL          string      `json:"agent_url"`
	AgentTokenEnc     string      `json:"-"`
	InternetInterface string      `json:"internet_interface"`
	Enabled           bool        `json:"enabled"`
	Status            string      `json:"status"`
	LastSeen          time.Time   `json:"last_seen"`
	Metrics           NodeMetrics `json:"metrics"`
	CreatedAt         time.Time   `json:"created_at"`
}

type Tunnel struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	SourceNodeID      string    `json:"source_node_id"`
	DestinationNodeID string    `json:"destination_node_id"`
	Transport         string    `json:"transport"`
	Profile           string    `json:"profile"`
	Ports             []int     `json:"ports"`
	MTU               int       `json:"mtu"`
	CIDR              string    `json:"cidr"`
	Interface         string    `json:"interface"`
	Status            string    `json:"status"`
	VNI               int       `json:"vni"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Inbound struct {
	ID                           string    `json:"id"`
	Name                         string    `json:"name"`
	Protocol                     string    `json:"protocol"`
	NodeID                       string    `json:"node_id"`
	Listen                       string    `json:"listen"`
	Port                         int       `json:"port"`
	Transport                    string    `json:"transport"`
	TLSMode                      string    `json:"tls_mode"`
	Enabled                      bool      `json:"enabled"`
	Status                       string    `json:"status"`
	Remark                       string    `json:"remark"`
	Path                         string    `json:"path"`
	Host                         string    `json:"host"`
	ServiceName                  string    `json:"service_name"`
	ServerName                   string    `json:"server_name"`
	CertFile                     string    `json:"cert_file"`
	KeyFile                      string    `json:"key_file"`
	RealityDest                  string    `json:"reality_dest"`
	RealityServerNames           []string  `json:"reality_server_names"`
	RealityPrivateKeyEnc         string    `json:"-"`
	RealityPublicKey             string    `json:"reality_public_key"`
	RealityShortIDs              []string  `json:"reality_short_ids"`
	RealityFingerprint           string    `json:"reality_fingerprint"`
	ShadowsocksMethod            string    `json:"shadowsocks_method"`
	ShadowsocksServerPasswordEnc string    `json:"-"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

type InboundClient struct {
	ID            string    `json:"id"`
	InboundID     string    `json:"inbound_id"`
	UserID        string    `json:"user_id"`
	Email         string    `json:"email"`
	CredentialEnc string    `json:"-"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
}

type Outbound struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	NodeID      string    `json:"node_id"`
	Tag         string    `json:"tag"`
	Protocol    string    `json:"protocol"`
	Address     string    `json:"address"`
	Port        int       `json:"port"`
	Username    string    `json:"username"`
	PasswordEnc string    `json:"-"`
	Enabled     bool      `json:"enabled"`
	Remark      string    `json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RoutingRule struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	NodeID     string    `json:"node_id"`
	Priority   int       `json:"priority"`
	Enabled    bool      `json:"enabled"`
	InboundIDs []string  `json:"inbound_ids"`
	UserIDs    []string  `json:"user_ids"`
	Domains    []string  `json:"domains"`
	IPs        []string  `json:"ips"`
	Ports      string    `json:"ports"`
	Network    string    `json:"network"`
	Protocols  []string  `json:"protocols"`
	OutboundID string    `json:"outbound_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type PortForward struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	Protocol   string    `json:"protocol"`
	ListenPort int       `json:"listen_port"`
	DestIP     string    `json:"dest_ip"`
	DestPort   int       `json:"dest_port"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
}

type VPNPeer struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	NodeID          string    `json:"node_id"`
	DeviceName      string    `json:"device_name"`
	Interface       string    `json:"interface"`
	Address         string    `json:"address"`
	PublicKey       string    `json:"public_key"`
	PrivateKeyEnc   string    `json:"-"`
	ConfigEnc       string    `json:"-"`
	ServerPublicKey string    `json:"server_public_key"`
	Endpoint        string    `json:"endpoint"`
	Enabled         bool      `json:"enabled"`
	RXBytes         int64     `json:"rx_bytes"`
	TXBytes         int64     `json:"tx_bytes"`
	RXBaseBytes     int64     `json:"rx_base_bytes"`
	TXBaseBytes     int64     `json:"tx_base_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}

type AuditEntry struct {
	ID        string    `json:"id"`
	AdminID   string    `json:"admin_id"`
	AdminName string    `json:"admin_name"`
	Action    string    `json:"action"`
	Object    string    `json:"object"`
	RemoteIP  string    `json:"remote_ip"`
	CreatedAt time.Time `json:"created_at"`
}

type State struct {
	Schema         int               `json:"schema"`
	Admins         []Admin           `json:"admins"`
	Plans          []Plan            `json:"plans"`
	Users          []User            `json:"users"`
	Nodes          []Node            `json:"nodes"`
	Tunnels        []Tunnel          `json:"tunnels"`
	Inbounds       []Inbound         `json:"inbounds"`
	InboundClients []InboundClient   `json:"inbound_clients"`
	Outbounds      []Outbound        `json:"outbounds"`
	RoutingRules   []RoutingRule     `json:"routing_rules"`
	Forwards       []PortForward     `json:"forwards"`
	VPNPeers       []VPNPeer         `json:"vpn_peers"`
	Audit          []AuditEntry      `json:"audit"`
	Settings       map[string]string `json:"settings"`
}

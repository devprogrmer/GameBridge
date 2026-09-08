// SPDX-License-Identifier: AGPL-3.0-only
package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Config struct{ Listen, Token, TLSCert, TLSKey, Version string }
type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(c Config) (*Server, error) {
	if len(c.Token) < 24 {
		return nil, errors.New("GAMEBRIDGE_AGENT_TOKEN must be at least 24 characters")
	}
	s := &Server{cfg: c, mux: http.NewServeMux()}
	s.routes()
	s.registerXrayRoutes()
	s.registerPhase3Routes()
	return s, nil
}
func (s *Server) routes() {
	s.mux.HandleFunc("/v1/status", s.auth(s.handleStatus))
	s.mux.HandleFunc("/v1/tunnels", s.auth(s.handleTunnels))
	s.mux.HandleFunc("/v1/tunnels/", s.auth(s.handleTunnelItem))
	s.mux.HandleFunc("/v1/logs", s.auth(s.handleLogs))
	s.mux.HandleFunc("/v1/forwards", s.auth(s.handleForwards))
	s.mux.HandleFunc("/v1/forwards/", s.auth(s.handleForwardItem))
	s.mux.HandleFunc("/v1/wireguard/server", s.auth(s.handleWGServer))
	s.mux.HandleFunc("/v1/wireguard/peer", s.auth(s.handleWGPeer))
	s.mux.HandleFunc("/v1/wireguard/stats", s.auth(s.handleWGStats))
	s.mux.HandleFunc("/v1/wireguard/outbounds/sync", s.auth(s.handleWGOutboundSync))
	s.mux.HandleFunc("/v1/tor/outbounds/sync", s.auth(s.handleTorOutboundSync))
	s.mux.HandleFunc("/v1/tor/status", s.auth(s.handleTorStatus))
	s.mux.HandleFunc("/v1/openvpn/outbounds/sync", s.auth(s.handleOpenVPNOutboundSync))
	s.mux.HandleFunc("/v1/openvpn/status", s.auth(s.handleOpenVPNStatus))
}
func (s *Server) auth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.cfg.Token {
			jsonError(w, 401, "unauthorized")
			return
		}
		fn(w, r)
	}
}
func (s *Server) ListenAndServe() error {
	log.Printf("GameBridge Agent %s listening on %s", s.cfg.Version, s.cfg.Listen)
	if s.cfg.TLSCert != "" && s.cfg.TLSKey != "" {
		return http.ListenAndServeTLS(s.cfg.Listen, s.cfg.TLSCert, s.cfg.TLSKey, s.mux)
	}
	return http.ListenAndServe(s.cfg.Listen, s.mux)
}
func jsonWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonWrite(w, status, map[string]string{"error": msg})
}
func decodeJSON(r *http.Request, d any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(d)
}
func runOutput(name string, args ...string) string {
	b, _ := exec.Command(name, args...).CombinedOutput()
	return string(b)
}
func run(name string, args ...string) error {
	b, e := exec.Command(name, args...).CombinedOutput()
	if e != nil {
		return fmt.Errorf("%s: %s", e, strings.TrimSpace(string(b)))
	}
	return nil
}

type Metrics struct {
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

func collectMetrics() Metrics {
	m := Metrics{}
	m.Hostname, _ = os.Hostname()
	m.Kernel = strings.TrimSpace(runOutput("uname", "-r"))
	m.CoreVersion = strings.TrimSpace(runOutput("/usr/local/bin/gamebridge-core", "version"))
	if b, e := os.ReadFile("/proc/uptime"); e == nil {
		fmt.Sscanf(string(b), "%f", &m.UptimeSeconds)
	}
	if b, e := os.ReadFile("/proc/loadavg"); e == nil {
		fmt.Sscanf(string(b), "%f %f %f", &m.Load1, &m.Load5, &m.Load15)
	}
	if f, e := os.Open("/proc/meminfo"); e == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var k string
			var v uint64
			fmt.Sscanf(sc.Text(), "%s %d", &k, &v)
			switch strings.TrimSuffix(k, ":") {
			case "MemTotal":
				m.MemoryTotal = v * 1024
			case "MemAvailable":
				m.MemoryAvailable = v * 1024
			}
		}
		f.Close()
	}
	m.DiskTotal, m.DiskFree = diskUsage("/")

	if f, e := os.Open("/proc/net/dev"); e == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.Contains(line, ":") {
				continue
			}
			p := strings.Fields(strings.Replace(line, ":", " ", 1))
			if len(p) >= 10 && p[0] != "lo" {
				rx, _ := strconv.ParseUint(p[1], 10, 64)
				tx, _ := strconv.ParseUint(p[9], 10, 64)
				m.NetworkRX += rx
				m.NetworkTX += tx
			}
		}
		f.Close()
	}
	return m
}
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	jsonWrite(w, 200, map[string]any{"version": s.cfg.Version, "metrics": collectMetrics(), "tunnels": listTunnels()})
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,40}$`)

type TunnelSpec struct {
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	Role                  string `json:"role"`
	Transport             string `json:"transport"`
	PeerHost              string `json:"peer_host"`
	Ports                 []int  `json:"ports"`
	Interface             string `json:"interface"`
	LocalCIDR             string `json:"local_cidr"`
	PeerIP                string `json:"peer_ip"`
	MTU                   int    `json:"mtu"`
	KeyHex                string `json:"key_hex"`
	Profile               string `json:"profile"`
	DuplicateSmallPackets bool   `json:"duplicate_small_packets"`
	KCPDataShards         int    `json:"kcp_data_shards"`
	KCPParityShards       int    `json:"kcp_parity_shards"`
	ICMPID                int    `json:"icmp_id"`
	ICMPPollMS            int    `json:"icmp_poll_ms"`
	ICMPBurst             int    `json:"icmp_burst"`
	NAT                   bool   `json:"nat"`
	InternetInterface     string `json:"internet_interface"`
	LocalPublic           string `json:"local_public"`
	RemotePublic          string `json:"remote_public"`
	VNI                   int    `json:"vni"`
}

func (s *Server) handleTunnels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonWrite(w, 200, listTunnels())
	case http.MethodPost:
		var x TunnelSpec
		if e := decodeJSON(r, &x); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		if !safeName.MatchString(x.Name) {
			jsonError(w, 400, "invalid name")
			return
		}
		var e error
		if x.Kind == "kernel" {
			e = createKernelTunnel(x)
		} else {
			e = createUserspaceTunnel(x)
		}
		if e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		jsonWrite(w, 201, map[string]any{"ok": true, "name": x.Name})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleTunnelItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/tunnels/")
	p := strings.Split(strings.Trim(rest, "/"), "/")
	name := p[0]
	if !safeName.MatchString(name) {
		jsonError(w, 400, "invalid name")
		return
	}
	if r.Method == http.MethodDelete {
		_ = deleteTunnel(name)
		jsonWrite(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method != http.MethodPost || len(p) != 2 {
		jsonError(w, 405, "method not allowed")
		return
	}
	action := p[1]
	if action == "status" {
		jsonWrite(w, 200, tunnelStatus(name))
		return
	}
	if action != "start" && action != "stop" && action != "restart" {
		jsonError(w, 400, "bad action")
		return
	}
	unit := detectTunnelUnit(name)
	if unit == "" {
		jsonError(w, 404, "tunnel not found")
		return
	}
	if e := run("systemctl", action, unit); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	jsonWrite(w, 200, tunnelStatus(name))
}
func createUserspaceTunnel(x TunnelSpec) error {
	if len(x.KeyHex) != 64 {
		return errors.New("key_hex must be 64 hex chars")
	}
	cfg := map[string]any{"name": x.Name, "role": x.Role, "transport": x.Transport, "listen_host": "0.0.0.0", "peer_host": x.PeerHost, "ports": x.Ports, "interface": x.Interface, "local_cidr": x.LocalCIDR, "peer_ip": x.PeerIP, "mtu": x.MTU, "key_hex": x.KeyHex, "profile": x.Profile, "duplicate_small_packets": x.DuplicateSmallPackets, "duplicate_threshold": 512, "kcp_data_shards": x.KCPDataShards, "kcp_parity_shards": x.KCPParityShards, "icmp_id": x.ICMPID, "icmp_poll_ms": x.ICMPPollMS, "icmp_burst": x.ICMPBurst, "nat": x.NAT, "internet_interface": x.InternetInterface, "keepalive_ms": 1000, "path_timeout_ms": 5000}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if e := os.MkdirAll("/etc/gamebridge", 0700); e != nil {
		return e
	}
	if e := os.WriteFile("/etc/gamebridge/"+x.Name+".json", append(b, '\n'), 0600); e != nil {
		return e
	}
	_ = run("systemctl", "daemon-reload")
	return run("systemctl", "enable", "--now", "gamebridge@"+x.Name+".service")
}
func q(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
func createKernelTunnel(x TunnelSpec) error {
	if x.Interface == "" {
		x.Interface = "gbk0"
	}
	if x.MTU == 0 {
		x.MTU = 1400
	}
	if x.VNI == 0 {
		x.VNI = 100
	}
	var add string
	switch x.Transport {
	case "gre", "ipip":
		add = fmt.Sprintf("ip tunnel add %s mode %s local %s remote %s ttl 255", q(x.Interface), x.Transport, q(x.LocalPublic), q(x.RemotePublic))
	case "sit":
		add = fmt.Sprintf("ip tunnel add %s mode sit local %s remote %s ttl 255", q(x.Interface), q(x.LocalPublic), q(x.RemotePublic))
	case "gre6":
		add = fmt.Sprintf("ip -6 tunnel add %s mode ip6gre local %s remote %s", q(x.Interface), q(x.LocalPublic), q(x.RemotePublic))
	case "geneve":
		add = fmt.Sprintf("ip link add %s type geneve id %d remote %s dstport 6081", q(x.Interface), x.VNI, q(x.RemotePublic))
	case "vxlan":
		add = fmt.Sprintf("ip link add %s type vxlan id %d remote %s local %s dstport 4789", q(x.Interface), x.VNI, q(x.RemotePublic), q(x.LocalPublic))
	default:
		return errors.New("unsupported kernel transport")
	}
	addr := "ip addr add " + q(x.LocalCIDR) + " dev " + q(x.Interface)
	if strings.Contains(x.LocalCIDR, ":") {
		addr = "ip -6 addr add " + q(x.LocalCIDR) + " dev " + q(x.Interface)
	}
	script := fmt.Sprintf("#!/usr/bin/env bash\nset -e\ncase \"${1:-up}\" in\nup)\n ip link del %s 2>/dev/null || true\n %s\n %s\n ip link set %s mtu %d up\n ;;\ndown) ip link del %s 2>/dev/null || true ;;\n*) exit 2 ;;\nesac\n", q(x.Interface), add, addr, q(x.Interface), x.MTU, q(x.Interface))
	if e := os.MkdirAll("/etc/gamebridge/kernel", 0700); e != nil {
		return e
	}
	if e := os.WriteFile("/etc/gamebridge/kernel/"+x.Name+".sh", []byte(script), 0700); e != nil {
		return e
	}
	_ = run("systemctl", "daemon-reload")
	return run("systemctl", "enable", "--now", "gamebridge-kernel@"+x.Name+".service")
}
func detectTunnelUnit(name string) string {
	if _, e := os.Stat("/etc/gamebridge/" + name + ".json"); e == nil {
		return "gamebridge@" + name + ".service"
	}
	if _, e := os.Stat("/etc/gamebridge/kernel/" + name + ".sh"); e == nil {
		return "gamebridge-kernel@" + name + ".service"
	}
	return ""
}
func tunnelStatus(name string) map[string]any {
	unit := detectTunnelUnit(name)
	st := "missing"
	if unit != "" {
		st = strings.TrimSpace(runOutput("systemctl", "is-active", unit))
	}
	return map[string]any{"name": name, "unit": unit, "status": st}
}
func deleteTunnel(name string) error {
	if u := detectTunnelUnit(name); u != "" {
		_ = run("systemctl", "disable", "--now", u)
	}
	_ = os.Remove("/etc/gamebridge/" + name + ".json")
	_ = os.Remove("/etc/gamebridge/kernel/" + name + ".sh")
	return nil
}
func listTunnels() []map[string]any {
	seen := map[string]bool{}
	var out []map[string]any
	for _, pat := range []string{"/etc/gamebridge/*.json", "/etc/gamebridge/kernel/*.sh"} {
		m, _ := filepath.Glob(pat)
		for _, p := range m {
			name := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, tunnelStatus(name))
		}
	}
	return out
}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "gamebridge-agent.service"
	}
	if !regexp.MustCompile(`^[A-Za-z0-9@_.-]+\.service$`).MatchString(unit) {
		jsonError(w, 400, "invalid unit")
		return
	}
	lines := 200
	if v, _ := strconv.Atoi(r.URL.Query().Get("lines")); v > 0 && v <= 2000 {
		lines = v
	}
	jsonWrite(w, 200, map[string]string{"logs": runOutput("journalctl", "-u", unit, "-n", strconv.Itoa(lines), "--no-pager", "--output=short-iso")})
}

type Forward struct {
	ID         string `json:"id"`
	Protocol   string `json:"protocol"`
	ListenPort int    `json:"listen_port"`
	DestIP     string `json:"dest_ip"`
	DestPort   int    `json:"dest_port"`
}

func (s *Server) handleForwards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method not allowed")
		return
	}
	var f Forward
	if e := decodeJSON(r, &f); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	if !safeName.MatchString(f.ID) {
		jsonError(w, 400, "invalid id")
		return
	}
	if e := installForward(f); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	jsonWrite(w, 201, map[string]bool{"ok": true})
}
func (s *Server) handleForwardItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, 405, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/forwards/")
	if !safeName.MatchString(id) {
		jsonError(w, 400, "invalid id")
		return
	}
	_ = deleteForward(id)
	jsonWrite(w, 200, map[string]bool{"ok": true})
}
func forwardArgs(f Forward, op string) [][]string {
	protos := []string{f.Protocol}
	if f.Protocol == "both" {
		protos = []string{"udp", "tcp"}
	}
	var out [][]string
	for _, p := range protos {
		out = append(out, []string{"-t", "nat", op, "PREROUTING", "-p", p, "--dport", strconv.Itoa(f.ListenPort), "-j", "DNAT", "--to-destination", f.DestIP + ":" + strconv.Itoa(f.DestPort)})
		out = append(out, []string{op, "FORWARD", "-p", p, "-d", f.DestIP, "--dport", strconv.Itoa(f.DestPort), "-j", "ACCEPT"})
		out = append(out, []string{"-t", "nat", op, "POSTROUTING", "-p", p, "-d", f.DestIP, "--dport", strconv.Itoa(f.DestPort), "-j", "MASQUERADE"})
	}
	return out
}
func installForward(f Forward) error {
	if f.ListenPort < 1 || f.ListenPort > 65535 || f.DestPort < 1 || f.DestPort > 65535 {
		return errors.New("invalid port")
	}
	if f.Protocol != "udp" && f.Protocol != "tcp" && f.Protocol != "both" {
		return errors.New("invalid protocol")
	}
	_ = os.MkdirAll("/etc/gamebridge/forward-rules.d", 0700)
	b, _ := json.Marshal(f)
	_ = os.WriteFile("/etc/gamebridge/forward-rules.d/"+f.ID+".json", b, 0600)
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	for _, a := range forwardArgs(f, "-A") {
		check := append([]string{}, a...)
		for i := range check {
			if check[i] == "-A" {
				check[i] = "-C"
			}
		}
		if exec.Command("iptables", check...).Run() != nil {
			if e := run("iptables", a...); e != nil {
				return e
			}
		}
	}
	script := "#!/usr/bin/env bash\nset -e\n"
	for _, a := range forwardArgs(f, "-A") {
		var parts []string
		for _, x := range a {
			parts = append(parts, q(x))
		}
		check := append([]string{}, a...)
		for i := range check {
			if check[i] == "-A" {
				check[i] = "-C"
			}
		}
		var cp []string
		for _, x := range check {
			cp = append(cp, q(x))
		}
		script += "iptables " + strings.Join(cp, " ") + " 2>/dev/null || iptables " + strings.Join(parts, " ") + "\n"
	}
	if e := os.WriteFile("/etc/gamebridge/forward-rules.d/"+f.ID+".sh", []byte(script), 0700); e != nil {
		return e
	}
	return ensureForwardAggregator()
}
func ensureForwardAggregator() error {
	return os.WriteFile("/etc/gamebridge/forward-rules.sh", []byte("#!/usr/bin/env bash\nset -e\nfor f in /etc/gamebridge/forward-rules.d/*.sh; do [ -e \"$f\" ] || continue; bash \"$f\"; done\n"), 0700)
}
func deleteForward(id string) error {
	p := "/etc/gamebridge/forward-rules.d/" + id + ".json"
	if b, e := os.ReadFile(p); e == nil {
		var f Forward
		if json.Unmarshal(b, &f) == nil {
			for _, a := range forwardArgs(f, "-D") {
				_ = exec.Command("iptables", a...).Run()
			}
		}
	}
	_ = os.Remove(p)
	_ = os.Remove("/etc/gamebridge/forward-rules.d/" + id + ".sh")
	return nil
}

type WGServer struct {
	Interface         string `json:"interface"`
	Address           string `json:"address"`
	ListenPort        int    `json:"listen_port"`
	InternetInterface string `json:"internet_interface"`
}
type WGPeer struct {
	Interface           string `json:"interface"`
	ClientAddress       string `json:"client_address"`
	Endpoint            string `json:"endpoint"`
	DNS                 string `json:"dns"`
	PersistentKeepalive int    `json:"persistent_keepalive"`
}

func wgGen() (string, string, error) {
	priv := strings.TrimSpace(runOutput("wg", "genkey"))
	if priv == "" {
		return "", "", errors.New("wg genkey failed")
	}
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(priv + "\n")
	b, e := cmd.Output()
	if e != nil {
		return "", "", e
	}
	return priv, strings.TrimSpace(string(b)), nil
}
func wgPub(priv string) (string, error) {
	cmd := exec.Command("wg", "pubkey")
	cmd.Stdin = strings.NewReader(priv + "\n")
	b, e := cmd.Output()
	return strings.TrimSpace(string(b)), e
}
func parsePriv(path string) string {
	b, _ := os.ReadFile(path)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "PrivateKey") {
			p := strings.SplitN(l, "=", 2)
			if len(p) == 2 {
				return strings.TrimSpace(p[1])
			}
		}
	}
	return ""
}
func ensureWG(x WGServer) (string, error) {
	if !safeName.MatchString(x.Interface) {
		return "", errors.New("invalid interface")
	}
	if x.ListenPort == 0 {
		x.ListenPort = 51820
	}
	if x.InternetInterface == "" {
		x.InternetInterface = "eth0"
	}
	path := "/etc/wireguard/" + x.Interface + ".conf"
	_ = os.MkdirAll("/etc/wireguard", 0700)
	if _, e := os.Stat(path); errors.Is(e, os.ErrNotExist) {
		priv, pub, e := wgGen()
		if e != nil {
			return "", e
		}
		cfg := fmt.Sprintf("[Interface]\nAddress = %s\nListenPort = %d\nPrivateKey = %s\nSaveConfig = true\nPostUp = iptables -A FORWARD -i %%i -j ACCEPT; iptables -A FORWARD -o %%i -j ACCEPT; iptables -t nat -A POSTROUTING -o %s -j MASQUERADE\nPostDown = iptables -D FORWARD -i %%i -j ACCEPT; iptables -D FORWARD -o %%i -j ACCEPT; iptables -t nat -D POSTROUTING -o %s -j MASQUERADE\n", x.Address, x.ListenPort, priv, x.InternetInterface, x.InternetInterface)
		if e = os.WriteFile(path, []byte(cfg), 0600); e != nil {
			return "", e
		}
		_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
		if e = run("systemctl", "enable", "--now", "wg-quick@"+x.Interface+".service"); e != nil {
			return "", e
		}
		return pub, nil
	}
	priv := parsePriv(path)
	if priv == "" {
		return "", errors.New("cannot read server private key")
	}
	pub, e := wgPub(priv)
	_ = run("systemctl", "enable", "--now", "wg-quick@"+x.Interface+".service")
	return pub, e
}
func (s *Server) handleWGServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method not allowed")
		return
	}
	var x WGServer
	if e := decodeJSON(r, &x); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	pub, e := ensureWG(x)
	if e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	jsonWrite(w, 200, map[string]any{"public_key": pub, "interface": x.Interface, "listen_port": x.ListenPort})
}
func (s *Server) handleWGPeer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var x WGPeer
		if e := decodeJSON(r, &x); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		resp, e := addWGPeer(x)
		if e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		jsonWrite(w, 201, resp)
	case http.MethodDelete:
		iface := r.URL.Query().Get("interface")
		pub := r.URL.Query().Get("public_key")
		if !safeName.MatchString(iface) || pub == "" {
			jsonError(w, 400, "invalid request")
			return
		}
		if e := run("wg", "set", iface, "peer", pub, "remove"); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		_ = exec.Command("wg-quick", "save", iface).Run()
		jsonWrite(w, 200, map[string]bool{"ok": true})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func addWGPeer(x WGPeer) (map[string]any, error) {
	if !safeName.MatchString(x.Interface) {
		return nil, errors.New("invalid interface")
	}
	priv, pub, e := wgGen()
	if e != nil {
		return nil, e
	}
	psk := strings.TrimSpace(runOutput("wg", "genpsk"))
	if psk == "" {
		return nil, errors.New("wg genpsk failed")
	}
	tmp, e := os.CreateTemp("", "gb-psk-*")
	if e != nil {
		return nil, e
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.WriteString(psk + "\n")
	_ = tmp.Close()
	_ = os.Chmod(tmp.Name(), 0600)
	ip, _, e := net.ParseCIDR(x.ClientAddress)
	if e != nil {
		return nil, e
	}
	allowed := ip.String() + "/32"
	if e = run("wg", "set", x.Interface, "peer", pub, "preshared-key", tmp.Name(), "allowed-ips", allowed); e != nil {
		return nil, e
	}
	_ = exec.Command("wg-quick", "save", x.Interface).Run()
	serverPriv := parsePriv("/etc/wireguard/" + x.Interface + ".conf")
	serverPub, e := wgPub(serverPriv)
	if e != nil {
		return nil, e
	}
	keep := x.PersistentKeepalive
	if keep == 0 {
		keep = 25
	}
	cfg := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\nDNS = %s\n\n[Peer]\nPublicKey = %s\nPresharedKey = %s\nEndpoint = %s\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = %d\n", priv, x.ClientAddress, x.DNS, serverPub, psk, x.Endpoint, keep)
	return map[string]any{"private_key": priv, "public_key": pub, "server_public_key": serverPub, "config": cfg}, nil
}

type wgStat struct {
	Interface string `json:"interface"`
	PublicKey string `json:"public_key"`
	RXBytes   int64  `json:"rx_bytes"`
	TXBytes   int64  `json:"tx_bytes"`
}

func (s *Server) handleWGStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	out := runOutput("wg", "show", "all", "dump")
	var peers []wgStat
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseInt(f[6], 10, 64)
		tx, _ := strconv.ParseInt(f[7], 10, 64)
		peers = append(peers, wgStat{Interface: f[0], PublicKey: f[1], RXBytes: rx, TXBytes: tx})
	}
	jsonWrite(w, 200, map[string]any{"peers": peers})
}

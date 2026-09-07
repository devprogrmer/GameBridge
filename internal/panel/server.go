// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

//go:embed web/*
var webFS embed.FS

type Config struct {
	Listen, StatePath, SessionKeyPath, MasterKeyPath string
	CookieSecure                                     bool
}
type Server struct {
	cfg        Config
	store      *Store
	crypt      *Crypt
	sessionKey []byte
	httpClient *http.Client
	mux        *http.ServeMux
}
type ctxKey string

const authKey ctxKey = "auth"

type authInfo struct {
	AdminID  string `json:"admin_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func New(cfg Config) (*Server, error) {
	st, e := OpenStore(cfg.StatePath)
	if e != nil {
		return nil, e
	}
	sk, e := ensureKey(cfg.SessionKeyPath)
	if e != nil {
		return nil, e
	}
	mk, e := ensureKey(cfg.MasterKeyPath)
	if e != nil {
		return nil, e
	}
	s := &Server{cfg: cfg, store: st, crypt: NewCrypt(mk), sessionKey: sk, httpClient: &http.Client{Timeout: 10 * time.Second}, mux: http.NewServeMux()}
	s.routes()
	go s.background()
	return s, nil
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }
func (s *Server) ListenAndServe() error {
	log.Printf("GameBridge Panel listening on %s", s.cfg.Listen)
	return http.ListenAndServe(s.cfg.Listen, s.Handler())
}
func (s *Server) routes() {
	s.mux.HandleFunc("/api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("/api/setup", s.handleSetup)
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/sub/", s.handleSubscription)
	s.mux.Handle("/api/", s.auth(http.HandlerFunc(s.handleAPI)))
	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := r.URL.Path
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if r.URL.Path == "/" || (!strings.Contains(base, ".") && !strings.HasPrefix(r.URL.Path, "/api/")) {
			b, _ := fs.ReadFile(sub, "index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
			return
		}
		static.ServeHTTP(w, r)
	}))
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("gb_session")
		if e != nil {
			jsonError(w, 401, "authentication required")
			return
		}
		p, ok := verifySession(s.sessionKey, c.Value)
		if !ok {
			jsonError(w, 401, "invalid session")
			return
		}
		enabled := false
		_ = s.store.Read(func(st State) error {
			for _, a := range st.Admins {
				if a.ID == p.AdminID && a.Enabled {
					enabled = true
					break
				}
			}
			return nil
		})
		if !enabled {
			jsonError(w, 401, "account disabled")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			cc, e := r.Cookie("gb_csrf")
			if e != nil || cc.Value == "" || r.Header.Get("X-GB-CSRF") != cc.Value {
				jsonError(w, 403, "CSRF validation failed")
				return
			}
		}
		ctx := context.WithValue(r.Context(), authKey, authInfo{AdminID: p.AdminID, Username: p.Username, Role: p.Role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func rank(role string) int {
	switch role {
	case "owner":
		return 4
	case "admin":
		return 3
	case "operator":
		return 2
	case "viewer":
		return 1
	}
	return 0
}
func requireRole(r *http.Request, role string) error {
	a, _ := r.Context().Value(authKey).(authInfo)
	if rank(a.Role) < rank(role) {
		return errors.New("insufficient permissions")
	}
	return nil
}
func decodeJSON(r *http.Request, d any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(d)
}
func jsonWrite(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonWrite(w, status, map[string]any{"error": msg})
}
func remoteIP(r *http.Request) string {
	h, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return h
	}
	return r.RemoteAddr
}
func (s *Server) audit(r *http.Request, action, object string) {
	a, _ := r.Context().Value(authKey).(authInfo)
	_ = s.store.Update(func(st *State) error {
		st.Audit = append(st.Audit, AuditEntry{ID: randomHex(8), AdminID: a.AdminID, AdminName: a.Username, Action: action, Object: object, RemoteIP: remoteIP(r), CreatedAt: time.Now().UTC()})
		sortAudit(st)
		return nil
	})
}
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	needs := false
	_ = s.store.Read(func(st State) error { needs = len(st.Admins) == 0; return nil })
	jsonWrite(w, 200, map[string]bool{"needs_setup": needs})
}
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method not allowed")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if e := decodeJSON(r, &in); e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	in.Username = normalizeUsername(in.Username)
	if len(in.Username) < 3 {
		jsonError(w, 400, "username too short")
		return
	}
	h, e := hashPassword(in.Password)
	if e != nil {
		jsonError(w, 400, e.Error())
		return
	}
	e = s.store.Update(func(st *State) error {
		if len(st.Admins) != 0 {
			return errors.New("setup already completed")
		}
		st.Admins = append(st.Admins, Admin{ID: randomHex(12), Username: in.Username, PasswordHash: h, Role: "owner", Enabled: true, CreatedAt: time.Now().UTC()})
		return nil
	})
	if e != nil {
		jsonError(w, 409, e.Error())
		return
	}
	jsonWrite(w, 201, map[string]bool{"ok": true})
}
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method not allowed")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	if decodeJSON(r, &in) != nil {
		jsonError(w, 400, "invalid request")
		return
	}
	var a Admin
	found := false
	_ = s.store.Read(func(st State) error {
		for _, x := range st.Admins {
			if x.Username == normalizeUsername(in.Username) {
				a = x
				found = true
				break
			}
		}
		return nil
	})
	if !found || !a.Enabled || !verifyPassword(a.PasswordHash, in.Password) || !verifyTOTP(a.TOTPSecret, in.TOTP, time.Now()) {
		time.Sleep(250 * time.Millisecond)
		jsonError(w, 401, "invalid credentials")
		return
	}
	tok, _ := signSession(s.sessionKey, sessionPayload{AdminID: a.ID, Username: a.Username, Role: a.Role, Exp: time.Now().Add(12 * time.Hour).Unix()})
	csrf := randomHex(24)
	http.SetCookie(w, &http.Cookie{Name: "gb_session", Value: tok, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	http.SetCookie(w, &http.Cookie{Name: "gb_csrf", Value: csrf, Path: "/", Secure: s.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	jsonWrite(w, 200, map[string]any{"username": a.Username, "role": a.Role, "totp_enabled": a.TOTPSecret != ""})
}
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "gb_session", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "gb_csrf", Path: "/", MaxAge: -1})
	jsonWrite(w, 200, map[string]bool{"ok": true})
}
func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case p == "me":
		a, _ := r.Context().Value(authKey).(authInfo)
		jsonWrite(w, 200, a)
	case p == "dashboard":
		s.handleDashboard(w, r)
	case p == "users":
		s.handleUsers(w, r)
	case strings.HasPrefix(p, "users/"):
		s.handleUserItem(w, r, strings.TrimPrefix(p, "users/"))
	case p == "plans":
		s.handlePlans(w, r)
	case strings.HasPrefix(p, "plans/"):
		s.handlePlanItem(w, r, strings.TrimPrefix(p, "plans/"))
	case p == "nodes":
		s.handleNodes(w, r)
	case strings.HasPrefix(p, "nodes/"):
		s.handleNodeItem(w, r, strings.TrimPrefix(p, "nodes/"))
	case p == "inbounds":
		s.handleInbounds(w, r)
	case strings.HasPrefix(p, "inbounds/"):
		s.handleInboundItem(w, r, strings.TrimPrefix(p, "inbounds/"))
	case p == "tunnels":
		s.handleTunnels(w, r)
	case strings.HasPrefix(p, "tunnels/"):
		s.handleTunnelItem(w, r, strings.TrimPrefix(p, "tunnels/"))
	case p == "forwards":
		s.handleForwards(w, r)
	case strings.HasPrefix(p, "forwards/"):
		s.handleForwardItem(w, r, strings.TrimPrefix(p, "forwards/"))
	case p == "logs":
		s.handleLogs(w, r)
	case p == "audit":
		s.handleAudit(w, r)
	case p == "admins":
		s.handleAdmins(w, r)
	case strings.HasPrefix(p, "admins/"):
		s.handleAdminItem(w, r, strings.TrimPrefix(p, "admins/"))
	case p == "settings":
		s.handleSettings(w, r)
	case p == "sync-traffic":
		if requireRole(r, "operator") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		s.syncWireGuard()
		jsonWrite(w, 200, map[string]bool{"ok": true})
	default:
		jsonError(w, 404, "not found")
	}
}
func firstAudit(a []AuditEntry, n int) []AuditEntry {
	if len(a) < n {
		n = len(a)
	}
	return a[:n]
}
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	var out map[string]any
	_ = s.store.Read(func(st State) error {
		online, active := 0, 0
		var traffic int64
		for _, n := range st.Nodes {
			if n.Status == "online" {
				online++
			}
		}
		for _, t := range st.Tunnels {
			if t.Status == "online" || t.Status == "created" {
				active++
			}
		}
		for _, u := range st.Users {
			traffic += u.TrafficUsedBytes
		}
		out = map[string]any{"users": len(st.Users), "nodes": len(st.Nodes), "nodes_online": online, "tunnels": len(st.Tunnels), "active_tunnels": active, "plans": len(st.Plans), "traffic_bytes": traffic, "nodes_detail": st.Nodes, "recent_audit": firstAudit(st.Audit, 8)}
		return nil
	})
	jsonWrite(w, 200, out)
}
func defaultString(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x []User
		_ = s.store.Read(func(st State) error { x = st.Users; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in User
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		in.Username = normalizeUsername(in.Username)
		if in.Username == "" {
			jsonError(w, 400, "username required")
			return
		}
		in.ID = randomHex(12)
		in.SubscriptionToken = randomHex(24)
		in.Status = defaultString(in.Status, "active")
		in.CreatedAt = time.Now().UTC()
		in.UpdatedAt = in.CreatedAt
		e := s.store.Update(func(st *State) error {
			for _, u := range st.Users {
				if u.Username == in.Username {
					return errors.New("username exists")
				}
			}
			maybeSetPlanExpiry(st, &in)
			st.Users = append(st.Users, in)
			return nil
		})
		if e != nil {
			jsonError(w, 409, e.Error())
			return
		}
		s.audit(r, "create", "user:"+in.Username)
		jsonWrite(w, 201, in)
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleUserItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "wireguard" {
		s.handleUserWireGuard(w, r, id)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var u *User
		var peers []VPNPeer
		_ = s.store.Read(func(st State) error {
			if x := findUser(&st, id); x != nil {
				c := *x
				u = &c
			}
			for _, p := range st.VPNPeers {
				if p.UserID == id {
					peers = append(peers, p)
				}
			}
			return nil
		})
		if u == nil {
			jsonError(w, 404, "user not found")
			return
		}
		jsonWrite(w, 200, map[string]any{"user": u, "vpn_peers": peers})
	case http.MethodDelete:
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var peers []VPNPeer
		_ = s.store.Read(func(st State) error {
			for _, p := range st.VPNPeers {
				if p.UserID == id {
					peers = append(peers, p)
				}
			}
			return nil
		})
		for _, p := range peers {
			_ = s.deleteRemotePeer(p)
		}
		_ = s.store.Update(func(st *State) error {
			st.Users = deleteByID(st.Users, id, func(x User) string { return x.ID })
			var keep []VPNPeer
			for _, p := range st.VPNPeers {
				if p.UserID != id {
					keep = append(keep, p)
				}
			}
			st.VPNPeers = keep
			return nil
		})
		s.audit(r, "delete", "user:"+id)
		jsonWrite(w, 200, map[string]bool{"ok": true})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handlePlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x []Plan
		_ = s.store.Read(func(st State) error { x = st.Plans; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in Plan
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		in.ID = randomHex(12)
		in.CreatedAt = time.Now().UTC()
		if in.DeviceLimit <= 0 {
			in.DeviceLimit = 1
		}
		in.Enabled = true
		_ = s.store.Update(func(st *State) error { st.Plans = append(st.Plans, in); return nil })
		s.audit(r, "create", "plan:"+in.Name)
		jsonWrite(w, 201, in)
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handlePlanItem(w http.ResponseWriter, r *http.Request, id string) {
	if requireRole(r, "admin") != nil {
		jsonError(w, 403, "forbidden")
		return
	}
	if r.Method != http.MethodDelete {
		jsonError(w, 405, "method not allowed")
		return
	}
	_ = s.store.Update(func(st *State) error {
		st.Plans = deleteByID(st.Plans, id, func(x Plan) string { return x.ID })
		return nil
	})
	s.audit(r, "delete", "plan:"+id)
	jsonWrite(w, 200, map[string]bool{"ok": true})
}
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x []Node
		_ = s.store.Read(func(st State) error { x = st.Nodes; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in struct {
			Name              string `json:"name"`
			Role              string `json:"role"`
			PublicIP          string `json:"public_ip"`
			AgentURL          string `json:"agent_url"`
			AgentToken        string `json:"agent_token"`
			InternetInterface string `json:"internet_interface"`
		}
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		if !strings.HasPrefix(in.AgentURL, "http://") && !strings.HasPrefix(in.AgentURL, "https://") {
			jsonError(w, 400, "agent URL must start with http:// or https://")
			return
		}
		enc, e := s.crypt.Seal(in.AgentToken)
		if e != nil {
			jsonError(w, 500, e.Error())
			return
		}
		n := Node{ID: randomHex(12), Name: in.Name, Role: in.Role, PublicIP: in.PublicIP, AgentURL: strings.TrimRight(in.AgentURL, "/"), AgentTokenEnc: enc, InternetInterface: defaultString(in.InternetInterface, "eth0"), Enabled: true, Status: "unknown", CreatedAt: time.Now().UTC()}
		_ = s.store.Update(func(st *State) error { st.Nodes = append(st.Nodes, n); return nil })
		go s.probeNode(n.ID)
		s.audit(r, "create", "node:"+n.Name)
		jsonWrite(w, 201, n)
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleNodeItem(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	id := parts[0]
	if len(parts) == 2 && parts[1] == "probe" && r.Method == http.MethodPost {
		go s.probeNode(id)
		jsonWrite(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodDelete {
		if requireRole(r, "admin") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		_ = s.store.Update(func(st *State) error {
			st.Nodes = deleteByID(st.Nodes, id, func(x Node) string { return x.ID })
			return nil
		})
		s.audit(r, "delete", "node:"+id)
		jsonWrite(w, 200, map[string]bool{"ok": true})
		return
	}
	jsonError(w, 405, "method not allowed")
}
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if requireRole(r, "admin") != nil {
		jsonError(w, 403, "forbidden")
		return
	}
	var x []AuditEntry
	_ = s.store.Read(func(st State) error { x = firstAudit(st.Audit, 300); return nil })
	jsonWrite(w, 200, x)
}
func (s *Server) handleAdmins(w http.ResponseWriter, r *http.Request) {
	if requireRole(r, "owner") != nil {
		jsonError(w, 403, "forbidden")
		return
	}
	switch r.Method {
	case http.MethodGet:
		var x []Admin
		_ = s.store.Read(func(st State) error { x = st.Admins; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPost:
		var in struct {
			Username   string `json:"username"`
			Password   string `json:"password"`
			Role       string `json:"role"`
			EnableTOTP bool   `json:"enable_totp"`
		}
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		if rank(in.Role) == 0 {
			jsonError(w, 400, "invalid role")
			return
		}
		h, e := hashPassword(in.Password)
		if e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		a := Admin{ID: randomHex(12), Username: normalizeUsername(in.Username), PasswordHash: h, Role: in.Role, Enabled: true, CreatedAt: time.Now().UTC()}
		if in.EnableTOTP {
			a.TOTPSecret = newTOTPSecret()
		}
		e = s.store.Update(func(st *State) error {
			for _, x := range st.Admins {
				if x.Username == a.Username {
					return errors.New("admin exists")
				}
			}
			st.Admins = append(st.Admins, a)
			return nil
		})
		if e != nil {
			jsonError(w, 409, e.Error())
			return
		}
		s.audit(r, "create", "admin:"+a.Username)
		jsonWrite(w, 201, map[string]any{"id": a.ID, "username": a.Username, "role": a.Role, "totp_secret": a.TOTPSecret})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleAdminItem(w http.ResponseWriter, r *http.Request, id string) {
	if requireRole(r, "owner") != nil {
		jsonError(w, 403, "forbidden")
		return
	}
	if r.Method != http.MethodDelete {
		jsonError(w, 405, "method not allowed")
		return
	}
	a, _ := r.Context().Value(authKey).(authInfo)
	if a.AdminID == id {
		jsonError(w, 400, "cannot delete current account")
		return
	}
	_ = s.store.Update(func(st *State) error {
		st.Admins = deleteByID(st.Admins, id, func(x Admin) string { return x.ID })
		return nil
	})
	s.audit(r, "delete", "admin:"+id)
	jsonWrite(w, 200, map[string]bool{"ok": true})
}
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var x map[string]string
		_ = s.store.Read(func(st State) error { x = st.Settings; return nil })
		jsonWrite(w, 200, x)
	case http.MethodPut:
		if requireRole(r, "owner") != nil {
			jsonError(w, 403, "forbidden")
			return
		}
		var in map[string]string
		if e := decodeJSON(r, &in); e != nil {
			jsonError(w, 400, e.Error())
			return
		}
		_ = s.store.Update(func(st *State) error {
			for k, v := range in {
				st.Settings[k] = v
			}
			return nil
		})
		s.audit(r, "update", "settings")
		jsonWrite(w, 200, map[string]bool{"ok": true})
	default:
		jsonError(w, 405, "method not allowed")
	}
}
func (s *Server) handleSubscription(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/sub/")
	var cfgs []string
	name := "GameBridge"
	_ = s.store.Read(func(st State) error {
		for _, u := range st.Users {
			if u.SubscriptionToken == token && u.Status == "active" {
				for _, p := range st.VPNPeers {
					if p.UserID == u.ID && p.Enabled {
						if c, e := s.crypt.Open(p.ConfigEnc); e == nil {
							cfgs = append(cfgs, c)
						}
					}
				}
			}
		}
		if st.Settings["site_name"] != "" {
			name = st.Settings["site_name"]
		}
		return nil
	})
	if len(cfgs) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# %s subscription\n\n%s\n", name, strings.Join(cfgs, "\n# --- next device ---\n"))
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: HTTP server, REST API, browser /ws push, and
//           embedded static dashboard assets.
//   0.0.7 - /api/hubs and hub counts in overview snapshot.

package server

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/auth"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/datastore"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/hubclient"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/relayprobe"
)

var browserUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Config configures the Monitor HTTP server.
type Config struct {
	BindAddr string
	StaticFS embed.FS
	Auth     *auth.Store
	Data     *datastore.Store
	Hub      *hubclient.Client
}

// Server is QuakeMeshMonitor's HTTP and WebSocket front end.
type Server struct {
	cfg        Config
	httpServer *http.Server
	addr       net.Addr

	mu       sync.Mutex
	browsers map[chan hubclient.Event]struct{}
}

// New creates a Server. Call Start to listen.
func New(cfg Config) *Server {
	s := &Server{
		cfg:      cfg,
		browsers: make(map[chan hubclient.Event]struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/change-password", s.handleChangePassword)
	mux.HandleFunc("/api/overview", s.requireAuth(s.handleOverview))
	mux.HandleFunc("/api/nodes", s.requireAuth(s.handleNodes))
	mux.HandleFunc("/api/hubs", s.requireAuth(s.handleHubs))
	mux.HandleFunc("/api/routes", s.requireAuth(s.handleRoutes))
	mux.HandleFunc("/api/relay-hubs", s.requireAuth(s.handleRelayHubs))
	mux.HandleFunc("/api/relay-hubs/", s.requireAuth(s.handleRelayHubAction))
	mux.HandleFunc("/ws", s.requireAuthWS(s.handleBrowserWS))
	mux.Handle("/", s.handleStatic())

	s.httpServer = &http.Server{Addr: cfg.BindAddr, Handler: mux}

	hubCh := make(chan hubclient.Event, 64)
	cfg.Hub.Subscribe(hubCh)
	go s.fanOutHubEvents(hubCh)

	return s
}

// Start begins serving HTTP.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", s.httpServer.Addr, err)
	}
	s.addr = ln.Addr()
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server: %v", err)
		}
	}()
	return nil
}

// Addr returns the bound address after Start.
func (s *Server) Addr() net.Addr { return s.addr }

// Close shuts down the HTTP server.
func (s *Server) Close(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) fanOutHubEvents(hubCh <-chan hubclient.Event) {
	for ev := range hubCh {
		s.mu.Lock()
		for ch := range s.browsers {
			select {
			case ch <- ev:
			default:
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) handleStatic() http.Handler {
	sub, err := fs.Sub(s.cfg.StaticFS, "static")
	if err != nil {
		panic(fmt.Sprintf("server: static fs: %v", err))
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFileFS(w, r, sub, "index.html")
			return
		}
		if r.URL.Path == "/login" {
			http.ServeFileFS(w, r, sub, "login.html")
			return
		}
		if r.URL.Path == "/change-password" {
			http.ServeFileFS(w, r, sub, "change-password.html")
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	clientKey := r.RemoteAddr
	token, mustChange, err := s.cfg.Auth.Login(body.Username, body.Password, clientKey)
	if errors.Is(err, auth.ErrLockedOut) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, auth.ErrInvalidCredentials) {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, map[string]any{
		"ok":                   true,
		"must_change_password": mustChange,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if token := sessionToken(r); token != "" {
		s.cfg.Auth.Logout(token)
	}
	clearSessionCookie(w)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, _, ok := s.session(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Auth.ChangePassword(username, body.CurrentPassword, body.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	o, err := s.cfg.Data.OverviewSnapshot()
	if err != nil {
		http.Error(w, "overview failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, o)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nodes, err := s.cfg.Data.Nodes()
	if err != nil {
		http.Error(w, "nodes query failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, nodes)
}

func (s *Server) handleHubs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hubs, err := s.cfg.Data.Hubs()
	if err != nil {
		http.Error(w, "hubs query failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, hubs)
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	routes, err := s.cfg.Data.Routes()
	if err != nil {
		http.Error(w, "routes query failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, routes)
}

func (s *Server) handleRelayHubs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hubs, err := s.cfg.Data.RelayHubs()
		if err != nil {
			http.Error(w, "relay hubs query failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, hubs)
	case http.MethodPost:
		var body struct {
			IP   string `json:"ip"`
			Port int    `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IP == "" || body.Port <= 0 {
			http.Error(w, "ip and port required", http.StatusBadRequest)
			return
		}
		hub, err := s.cfg.Data.AddRelayHub(body.IP, body.Port)
		if err != nil {
			http.Error(w, "add relay hub failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, hub)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRelayHubAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/relay-hubs/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	var hubID identity.NodeID
	idBytes, err := hex.DecodeString(parts[0])
	if err != nil || len(idBytes) != len(hubID) {
		http.Error(w, "invalid hub id", http.StatusBadRequest)
		return
	}
	copy(hubID[:], idBytes)

	if len(parts) == 2 && parts[1] == "probe" && r.Method == http.MethodPost {
		hub, err := s.cfg.Data.RelayHubByID(hubID)
		if err != nil {
			http.Error(w, "relay hub not found", http.StatusNotFound)
			return
		}
		probeErr := relayprobe.Probe(hub.IP, hub.Port, 5*time.Second)
		verifiedAt := time.Now()
		if probeErr == nil {
			_ = s.cfg.Data.MarkRelayHubVerified(hubID, verifiedAt)
		}
		writeJSON(w, map[string]any{
			"ok":            probeErr == nil,
			"error":         errString(probeErr),
			"last_verified": verifiedAt,
		})
		return
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.cfg.Data.RemoveRelayHub(hubID); err != nil {
			http.Error(w, "delete failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleBrowserWS(w http.ResponseWriter, r *http.Request) {
	conn, err := browserUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("server: browser ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	ch := make(chan hubclient.Event, 32)
	s.mu.Lock()
	s.browsers[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.browsers, ch)
		s.mu.Unlock()
	}()

	// Push an initial overview snapshot on connect.
	if o, err := s.cfg.Data.OverviewSnapshot(); err == nil {
		_ = conn.WriteJSON(map[string]any{
			"type":           "overview_snapshot",
			"total_nodes":    o.TotalNodes,
			"online_nodes":   o.OnlineNodes,
			"offline_nodes":  o.OfflineNodes,
			"total_hubs":     o.TotalHubs,
			"online_hubs":    o.OnlineHubs,
			"offline_hubs":   o.OfflineHubs,
			"route_count":    o.RouteCount,
			"dtn_depth":      o.DTNDepth,
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case ev := <-ch:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		}
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, mustChange, ok := s.session(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if mustChange {
			http.Error(w, auth.ErrMustChangePassword.Error(), http.StatusForbidden)
			return
		}
		r.Header.Set("X-QuakeMesh-User", username)
		next(w, r)
	}
}

func (s *Server) requireAuthWS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, mustChange, ok := s.session(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if mustChange {
			http.Error(w, auth.ErrMustChangePassword.Error(), http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) session(r *http.Request) (username string, mustChange bool, ok bool) {
	return s.cfg.Auth.ValidateSession(sessionToken(r))
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie(auth.SessionCookieName())
	if err != nil {
		return ""
	}
	return c.Value
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

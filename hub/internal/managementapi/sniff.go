// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   1.0.2 - Unauthenticated GET /sniff (+ /api/sniff) for MeshSniff discovery.

package managementapi

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// SniffConfig is advertised on GET /sniff for MeshSniff / LAN identity.
type SniffConfig struct {
	MeshID         string
	AppVersion     string
	HeartbeatPort  int
	ManagementPort int
	OGMPort        int
	DiscoveryPort  int
}

type sniffService struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	URL  string `json:"url,omitempty"`
}

// SetSniff installs the payload source for GET /sniff and GET /api/sniff.
func (s *Server) SetSniff(cfg SniffConfig) {
	s.sniffMu.Lock()
	defer s.sniffMu.Unlock()
	s.sniff = cfg
}

func (s *Server) handleSniff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.sniffMu.Lock()
	cfg := s.sniff
	s.sniffMu.Unlock()

	host := requestHost(r)
	mgmtPort := cfg.ManagementPort
	if mgmtPort <= 0 {
		mgmtPort, _ = portFromListenAddr(s.httpServer.Addr)
	}
	managementURL := ""
	if mgmtPort > 0 {
		managementURL = fmt.Sprintf("ws://%s/ws", net.JoinHostPort(host, strconv.Itoa(mgmtPort)))
	}

	var services []sniffService
	if cfg.HeartbeatPort > 0 {
		hbURL := fmt.Sprintf("http://%s/v1/heartbeat", net.JoinHostPort(host, strconv.Itoa(cfg.HeartbeatPort)))
		services = append(services, sniffService{
			Name: "QuakeMesh Hub Heartbeat",
			Port: cfg.HeartbeatPort,
			URL:  hbURL,
		})
	}
	if mgmtPort > 0 {
		services = append(services, sniffService{
			Name: "QuakeMesh Hub Management",
			Port: mgmtPort,
			URL:  managementURL,
		})
	}
	if cfg.OGMPort > 0 {
		services = append(services, sniffService{Name: "QuakeMesh Hub OGM", Port: cfg.OGMPort})
	}
	if cfg.DiscoveryPort > 0 {
		services = append(services, sniffService{Name: "QuakeMesh Hub Discovery", Port: cfg.DiscoveryPort})
	}

	out := map[string]any{
		"meshId":     cfg.MeshID,
		"name":       "QuakeMeshHub",
		"platform":   "quakemesh-hub",
		"appVersion": cfg.AppVersion,
		"software":   "QuakeMeshHub",
		"urls": map[string]string{
			"management": managementURL,
		},
		"services": services,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}

func requestHost(r *http.Request) string {
	h := r.Host
	if h == "" {
		return "127.0.0.1"
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return strings.Trim(h, "[]")
}

func portFromListenAddr(addr string) (int, error) {
	if addr == "" {
		return 0, fmt.Errorf("empty addr")
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}

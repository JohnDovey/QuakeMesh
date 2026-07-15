// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   1.0.2 - Unauthenticated GET /sniff (+ /api/sniff) for MeshSniff LAN discovery.

package nodeheartbeat

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/JohnDovey/QuakeMesh/core/identity"
)

type sniffService struct {
	Name string `json:"name"`
	Port int    `json:"port"`
	URL  string `json:"url,omitempty"`
}

func (s *Server) handleSniff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := requestHost(r)
	hbPort := s.cfg.HeartbeatPort
	if hbPort <= 0 {
		hbPort, _ = portFromListenAddr(s.cfg.ListenAddr)
	}
	heartbeatURL := ""
	if hbPort > 0 {
		heartbeatURL = fmt.Sprintf("http://%s/v1/heartbeat", net.JoinHostPort(host, strconv.Itoa(hbPort)))
	}

	var services []sniffService
	if hbPort > 0 {
		services = append(services, sniffService{
			Name: "QuakeMesh Hub Heartbeat",
			Port: hbPort,
			URL:  heartbeatURL,
		})
	}
	if s.cfg.ManagementPort > 0 {
		services = append(services, sniffService{
			Name: "QuakeMesh Hub Management",
			Port: s.cfg.ManagementPort,
		})
	}
	if s.cfg.OGMPort > 0 {
		services = append(services, sniffService{Name: "QuakeMesh Hub OGM", Port: s.cfg.OGMPort})
	}
	if s.cfg.DiscoveryPort > 0 {
		services = append(services, sniffService{Name: "QuakeMesh Hub Discovery", Port: s.cfg.DiscoveryPort})
	}

	meshID := ""
	if s.cfg.LocalHub != (identity.NodeID{}) {
		meshID = s.cfg.LocalHub.String()
	}
	out := map[string]any{
		"meshId":     meshID,
		"name":       "QuakeMeshHub",
		"platform":   "quakemesh-hub",
		"appVersion": s.cfg.AppVersion,
		"software":   "QuakeMeshHub",
		"urls": map[string]string{
			"heartbeat": heartbeatURL,
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

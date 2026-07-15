// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   1.0.2 - Unauthenticated GET /sniff (+ /api/sniff) for MeshSniff LAN discovery.

package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	port, _ := portFromListenAddr(s.cfg.BindAddr)
	if port <= 0 && s.addr != nil {
		port, _ = portFromListenAddr(s.addr.String())
	}
	dashURL := ""
	if port > 0 {
		dashURL = fmt.Sprintf("http://%s/", net.JoinHostPort(host, strconv.Itoa(port)))
	}

	meshID := ""
	if s.cfg.Data != nil {
		if id, err := s.cfg.Data.LocalHubID(); err == nil {
			meshID = id.String()
		}
	}

	services := []sniffService{}
	if port > 0 {
		services = append(services, sniffService{
			Name: "QuakeMesh Monitor",
			Port: port,
			URL:  dashURL,
		})
	}

	out := map[string]any{
		"meshId":     meshID,
		"name":       "QuakeMeshMonitor",
		"platform":   "quakemesh-monitor",
		"appVersion": s.cfg.AppVersion,
		"software":   "QuakeMeshMonitor",
		"urls": map[string]string{
			"dashboard": dashURL,
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

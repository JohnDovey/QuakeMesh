// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.17 - LAN HTTP heartbeat so mesh nodes (e.g. Android) appear in Monitor.
//   0.0.19 - optional lan_context on heartbeat for infrastructure segments.
//   0.0.22 - hub proximity on heartbeat; POST /v1/endorse for mesh nodes.

// Package nodeheartbeat accepts periodic presence reports from mesh nodes
// that are not yet speaking the hub OGM protocol (Phase 4 Android stub).
package nodeheartbeat

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lancontext"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
	"github.com/JohnDovey/QuakeMesh/core/trust"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// Notifier emits registry-visible changes to subscribers (management API).
type Notifier interface {
	NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus)
}

// SOSNotifier receives LAN SOS alerts for the Monitor event stream.
type SOSNotifier interface {
	SosAlertPublished(nodeID identity.NodeID, appID, topic string, payload []byte)
}

const (
	sosAppID = "net.quakemesh.sosbeacon"
	sosTopic = "sos"
)

// Config configures a heartbeat Server.
type Config struct {
	ListenAddr  string // e.g. "0.0.0.0:18085"; empty disables
	Registry    *registry.Registry
	Notifier    Notifier
	SOSNotifier SOSNotifier
	Segments    *lansegments.Store
	Trust       *trust.Store
	LocalHub    identity.NodeID
}

// Server accepts POST /v1/heartbeat from mesh nodes on the LAN.
type Server struct {
	cfg        Config
	httpServer *http.Server
	listener   net.Listener
}

// New creates a Server. Call Start to listen.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/heartbeat", s.handleHeartbeat)
	mux.HandleFunc("/v1/sos", s.handleSOS)
	mux.HandleFunc("/v1/endorse", s.handleEndorse)
	s.httpServer = &http.Server{Handler: mux}
	return s
}

// Start listens until Close.
func (s *Server) Start() error {
	if s.cfg.ListenAddr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("nodeheartbeat: listen %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("nodeheartbeat: %v", err)
		}
	}()
	return nil
}

// Addr returns the bound address after Start.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Close shuts down the server.
func (s *Server) Close() error {
	if s.httpServer == nil || s.listener == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		NodeID     string              `json:"node_id"`
		Lat        *float64            `json:"lat"`
		Lon        *float64            `json:"lon"`
		AccuracyM  *float64            `json:"accuracy_m"`
		LanContext *lancontext.Context `json:"lan_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	idBytes, err := hex.DecodeString(body.NodeID)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		http.Error(w, "invalid node_id", http.StatusBadRequest)
		return
	}
	var nodeID identity.NodeID
	copy(nodeID[:], idBytes)

	now := time.Now()
	_, err = RegisterPresence(s.cfg.Registry, s.cfg.Notifier, nodeID, body.Lat, body.Lon, now)
	if err != nil {
		http.Error(w, "registry failed", http.StatusInternalServerError)
		return
	}
	if body.LanContext != nil && s.cfg.Segments != nil {
		ctx := *body.LanContext
		if ctx.LocalIP == "" {
			ctx.LocalIP = lancontext.LocalIPFromRemoteAddr(r.RemoteAddr)
		}
		_ = s.cfg.Segments.RecordMembership(lansegments.EntityNode, nodeID, ctx, now)
	}
	if s.cfg.Trust != nil && s.cfg.LocalHub != (identity.NodeID{}) {
		_ = s.cfg.Trust.RecordProximity(s.cfg.LocalHub, nodeID, -55, trust.TransportHubDirect, now)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleSOS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		NodeID    string  `json:"node_id"`
		Text      string  `json:"text"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		AccuracyM float64 `json:"accuracy_m"`
		SentAt    int64   `json:"sent_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.NodeID == "" {
		http.Error(w, "node_id required", http.StatusBadRequest)
		return
	}
	if body.Text == "" {
		body.Text = "SOS — need assistance"
	}
	idBytes, err := hex.DecodeString(body.NodeID)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		http.Error(w, "invalid node_id", http.StatusBadRequest)
		return
	}
	var nodeID identity.NodeID
	copy(nodeID[:], idBytes)

	now := time.Now()
	var lat, lon *float64
	if body.Lat != 0 || body.Lon != 0 {
		lat, lon = &body.Lat, &body.Lon
	}
	_, err = RegisterPresence(s.cfg.Registry, s.cfg.Notifier, nodeID, lat, lon, now)
	if err != nil {
		http.Error(w, "registry failed", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(body)
	if s.cfg.SOSNotifier != nil {
		s.cfg.SOSNotifier.SosAlertPublished(nodeID, sosAppID, sosTopic, payload)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) handleEndorse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.Trust == nil {
		http.Error(w, "endorsements disabled", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		EndorserNodeID string `json:"endorser_node_id"`
		EndorsedNodeID string `json:"endorsed_node_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EndorserNodeID == "" || body.EndorsedNodeID == "" {
		http.Error(w, "endorser_node_id and endorsed_node_id required", http.StatusBadRequest)
		return
	}
	endorser, err := parseNodeID(body.EndorserNodeID)
	if err != nil {
		http.Error(w, "invalid endorser_node_id", http.StatusBadRequest)
		return
	}
	endorsed, err := parseNodeID(body.EndorsedNodeID)
	if err != nil {
		http.Error(w, "invalid endorsed_node_id", http.StatusBadRequest)
		return
	}
	if endorser == endorsed {
		http.Error(w, "cannot endorse self", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if err := s.cfg.Trust.EndorseWithHubContact(endorser, endorsed, now); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func parseNodeID(hexID string) (identity.NodeID, error) {
	idBytes, err := hex.DecodeString(hexID)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		return identity.NodeID{}, fmt.Errorf("invalid node id")
	}
	var nodeID identity.NodeID
	copy(nodeID[:], idBytes)
	return nodeID, nil
}

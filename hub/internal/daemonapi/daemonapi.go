// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10: local JSON HTTP API for mesh-sdk clients.

// Package daemonapi exposes Register/Send/Receive/Publish/Subscribe/DiscoverPeers
// over HTTP on a Unix domain socket (or TCP loopback for tests).
package daemonapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/banlist"
	"github.com/JohnDovey/QuakeMesh/core/identity"
)

// DTNSender enqueues mesh-routed point-to-point payloads.
type DTNSender interface {
	Enqueue(src, dst identity.NodeID, payload []byte) error
}

// PresenceNotifier is called when app presence changes.
type PresenceNotifier interface {
	AppPresenceChanged(nodeID identity.NodeID, appID, appVersion string)
}

// Config configures a Server.
type Config struct {
	SelfID     identity.NodeID
	ListenAddr string // "unix:/path" or "tcp:127.0.0.1:8084"
	Apps       *apppresence.Store
	Bans       *banlist.Store
	LocalHub   identity.NodeID
	Sender     DTNSender
	Notifier   PresenceNotifier
}

// Server is the local mesh-sdk daemon API.
type Server struct {
	cfg        Config
	httpServer *http.Server
	listener   net.Listener

	mu       sync.Mutex
	sessions map[string]session
	inbox    map[identity.NodeID][][]byte
	topics   map[string]map[string]chan []byte // topic -> sessionToken -> ch
}

type session struct {
	token      string
	nodeID     identity.NodeID
	appID      string
	appName    string
	appVersion string
}

// New creates a Server. Call Start to listen.
func New(cfg Config) *Server {
	s := &Server{
		cfg:      cfg,
		sessions: make(map[string]session),
		inbox:    make(map[identity.NodeID][][]byte),
		topics:   make(map[string]map[string]chan []byte),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/register", s.handleRegister)
	mux.HandleFunc("/v1/send", s.handleSend)
	mux.HandleFunc("/v1/receive", s.handleReceive)
	mux.HandleFunc("/v1/publish", s.handlePublish)
	mux.HandleFunc("/v1/subscribe", s.handleSubscribe)
	mux.HandleFunc("/v1/discover-peers", s.handleDiscoverPeers)
	s.httpServer = &http.Server{Handler: mux}
	return s
}

// Start listens and serves until Close.
func (s *Server) Start() error {
	network, addr, err := splitListenAddr(s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	if network == "unix" {
		if err := os.MkdirAll(filepath.Dir(addr), 0o755); err != nil {
			return fmt.Errorf("daemonapi: mkdir: %w", err)
		}
		_ = os.Remove(addr)
	}
	ln, err := net.Listen(network, addr)
	if err != nil {
		return fmt.Errorf("daemonapi: listen %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("daemonapi: %v", err)
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
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// DeliverLocal implements dtnengine.LocalDeliverer for bundles addressed to SelfID.
func (s *Server) DeliverLocal(src, dst identity.NodeID, payload []byte) error {
	if dst != s.cfg.SelfID {
		return fmt.Errorf("daemonapi: not local destination")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := append([]byte(nil), payload...)
	s.inbox[dst] = append(s.inbox[dst], cp)
	return nil
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		AppID        string   `json:"app_id"`
		AppName      string   `json:"app_name"`
		AppVersion   string   `json:"app_version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AppID == "" {
		http.Error(w, "app_id required", http.StatusBadRequest)
		return
	}
	if s.cfg.Bans != nil {
		blocked, err := s.cfg.Bans.IsLocallyEnforced(s.cfg.LocalHub, body.AppID, body.AppVersion)
		if err != nil {
			http.Error(w, "ban check failed", http.StatusInternalServerError)
			return
		}
		if blocked {
			http.Error(w, "app is banned on this hub", http.StatusForbidden)
			return
		}
	}
	token, err := newToken()
	if err != nil {
		http.Error(w, "token failed", http.StatusInternalServerError)
		return
	}
	sess := session{
		token:      token,
		nodeID:     s.cfg.SelfID,
		appID:      body.AppID,
		appName:    body.AppName,
		appVersion: body.AppVersion,
	}
	now := time.Now()
	if s.cfg.Apps != nil {
		if err := s.cfg.Apps.Upsert(s.cfg.SelfID, body.AppID, body.AppName, body.AppVersion, now); err != nil {
			http.Error(w, "presence failed", http.StatusInternalServerError)
			return
		}
	}
	s.mu.Lock()
	s.sessions[token] = sess
	s.mu.Unlock()
	if s.cfg.Notifier != nil {
		s.cfg.Notifier.AppPresenceChanged(s.cfg.SelfID, body.AppID, body.AppVersion)
	}
	writeJSON(w, map[string]string{
		"session_token": token,
		"node_id":       s.cfg.SelfID.String(),
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		DestNodeID string `json:"dest_node_id"`
		PayloadB64 string `json:"payload_b64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DestNodeID == "" {
		http.Error(w, "dest_node_id and payload_b64 required", http.StatusBadRequest)
		return
	}
	payload, err := base64.StdEncoding.DecodeString(body.PayloadB64)
	if err != nil {
		http.Error(w, "invalid payload_b64", http.StatusBadRequest)
		return
	}
	var dst identity.NodeID
	idBytes, err := hex.DecodeString(body.DestNodeID)
	if err != nil || len(idBytes) != len(dst) {
		http.Error(w, "invalid dest_node_id", http.StatusBadRequest)
		return
	}
	copy(dst[:], idBytes)
	if s.cfg.Sender == nil {
		http.Error(w, "send unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.cfg.Sender.Enqueue(sess.nodeID, dst, payload); err != nil {
		http.Error(w, "enqueue failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleReceive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	queue := s.inbox[sess.nodeID]
	var payload []byte
	if len(queue) > 0 {
		payload = queue[0]
		s.inbox[sess.nodeID] = queue[1:]
	}
	s.mu.Unlock()
	if payload == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, map[string]string{
		"payload_b64": base64.StdEncoding.EncodeToString(payload),
	})
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.sessionFromRequest(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Topic      string `json:"topic"`
		PayloadB64 string `json:"payload_b64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Topic == "" {
		http.Error(w, "topic and payload_b64 required", http.StatusBadRequest)
		return
	}
	payload, err := base64.StdEncoding.DecodeString(body.PayloadB64)
	if err != nil {
		http.Error(w, "invalid payload_b64", http.StatusBadRequest)
		return
	}
	s.broadcastTopic(body.Topic, payload)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := s.sessionFromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	topic := r.URL.Query().Get("topic")
	if topic == "" {
		http.Error(w, "topic required", http.StatusBadRequest)
		return
	}
	ch := make(chan []byte, 8)
	s.mu.Lock()
	if s.topics[topic] == nil {
		s.topics[topic] = make(map[string]chan []byte)
	}
	s.topics[topic][sess.token] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.topics[topic], sess.token)
		s.mu.Unlock()
	}()

	timeout := 25 * time.Second
	if d := r.URL.Query().Get("timeout"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil && parsed > 0 && parsed < 60*time.Second {
			timeout = parsed
		}
	}
	select {
	case payload := <-ch:
		writeJSON(w, map[string]string{
			"payload_b64": base64.StdEncoding.EncodeToString(payload),
		})
	case <-time.After(timeout):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	}
}

func (s *Server) handleDiscoverPeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.sessionFromRequest(r); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		http.Error(w, "app_id required", http.StatusBadRequest)
		return
	}
	constraint := r.URL.Query().Get("version_constraint")
	if s.cfg.Apps == nil {
		writeJSON(w, map[string][]string{"peers": nil})
		return
	}
	peers, err := s.cfg.Apps.DiscoverPeers(appID, constraint)
	if err != nil {
		http.Error(w, "discover failed", http.StatusInternalServerError)
		return
	}
	out := make([]string, 0, len(peers))
	for _, p := range peers {
		if s.cfg.Bans != nil {
			rec, ok, err := s.cfg.Apps.Get(p, appID)
			if err != nil {
				continue
			}
			if ok {
				blocked, err := s.cfg.Bans.IsLocallyEnforced(s.cfg.LocalHub, appID, rec.AppVersion)
				if err != nil || blocked {
					continue
				}
			}
		}
		out = append(out, p.String())
	}
	writeJSON(w, map[string][]string{"peers": out})
}

func (s *Server) broadcastTopic(topic string, payload []byte) {
	s.mu.Lock()
	subs := s.topics[topic]
	chans := make([]chan []byte, 0, len(subs))
	for _, ch := range subs {
		chans = append(chans, ch)
	}
	s.mu.Unlock()
	cp := append([]byte(nil), payload...)
	for _, ch := range chans {
		select {
		case ch <- cp:
		default:
		}
	}
}

func (s *Server) sessionFromRequest(r *http.Request) (session, bool) {
	token := r.Header.Get("X-Mesh-Session")
	if token == "" {
		token = r.URL.Query().Get("session_token")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	return sess, ok && token != ""
}

func newToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func splitListenAddr(addr string) (network, host string, err error) {
	if addr == "" {
		return "unix", "/tmp/quakemeshhub.sock", nil
	}
	if len(addr) > 5 && addr[:5] == "unix:" {
		return "unix", addr[5:], nil
	}
	if len(addr) > 4 && addr[:4] == "tcp:" {
		return "tcp", addr[4:], nil
	}
	return "", "", fmt.Errorf("daemonapi: listen addr must be unix: or tcp: prefix")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// DrainBody is used in tests.
func DrainBody(r *http.Request) { io.Copy(io.Discard, r.Body) } //nolint:errcheck

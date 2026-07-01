// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial loopback management API: /ws WebSocket event
//           stream, and an EventHandler adapter so it can be wired
//           directly into ogmengine. Start now actually records the
//           bound address (exposed via Addr) -- it was declared but
//           never assigned, nil-panicking any caller that used it.

// Package managementapi implements QuakeMeshHub's loopback management
// API: an HTTP server exposing a /ws WebSocket endpoint that streams
// wire.ManagementEvent (binary protobuf) to subscribers such as
// QuakeMeshMonitor. See "QuakeMeshHub" and "QuakeMeshMonitor" in
// /plan.md.
package managementapi

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/wire"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

var upgrader = websocket.Upgrader{
	// This is a loopback-only API (127.0.0.1:8083); Monitor connects as
	// a plain WebSocket client, not a browser tab making a cross-origin
	// request against this port, so origin checking does not apply.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server is QuakeMeshHub's loopback management API.
type Server struct {
	httpServer *http.Server
	addr       net.Addr

	mu      sync.Mutex
	clients map[chan []byte]struct{}
}

// New creates a Server that will bind to addr (e.g. "127.0.0.1:8083")
// once Start is called.
func New(addr string) *Server {
	s := &Server{clients: make(map[chan []byte]struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	s.httpServer = &http.Server{Addr: addr, Handler: mux}
	return s
}

// Start begins listening and serving in the background.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("managementapi: listen %s: %w", s.httpServer.Addr, err)
	}
	s.addr = ln.Addr()
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("managementapi: serve: %v", err)
		}
	}()
	return nil
}

// Addr returns the server's bound address. Only valid after Start.
func (s *Server) Addr() net.Addr {
	return s.addr
}

// Close shuts down the HTTP server and disconnects every client.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) addClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[ch] = struct{}{}
}

func (s *Server) removeClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, ch)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("managementapi: upgrade: %v", err)
		return
	}
	defer conn.Close()

	ch := make(chan []byte, 32)
	s.addClient(ch)
	defer s.removeClient(ch)

	// This channel is server-push only; the read loop exists solely to
	// detect client disconnects (and answer control frames, handled by
	// gorilla/websocket internally).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case payload := <-ch:
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
				return
			}
		}
	}
}

// Publish broadcasts event to every connected client. A client that
// isn't reading fast enough has this event dropped rather than blocking
// the publisher or the other clients.
func (s *Server) Publish(event *wire.ManagementEvent) {
	payload, err := proto.Marshal(event)
	if err != nil {
		log.Printf("managementapi: marshal event: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

// NodeStatusChanged implements ogmengine.EventHandler, letting a Server
// be wired directly in as an Engine's event handler.
func (s *Server) NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus) {
	s.Publish(&wire.ManagementEvent{
		EmittedAtUnixMs: time.Now().UnixMilli(),
		Event: &wire.ManagementEvent_NodeStatusChanged{
			NodeStatusChanged: &wire.NodeStatusChanged{
				NodeId: nodeID[:],
				Status: string(status),
			},
		},
	})
}

// RouteChanged implements ogmengine.EventHandler.
func (s *Server) RouteChanged(route registry.Route) {
	s.Publish(&wire.ManagementEvent{
		EmittedAtUnixMs: time.Now().UnixMilli(),
		Event: &wire.ManagementEvent_RouteChanged{
			RouteChanged: &wire.RouteChanged{
				DstNodeId:     route.Destination[:],
				NextHopNodeId: route.NextHop[:],
				Tq:            route.TQ,
				HopCount:      uint32(route.HopCount),
			},
		},
	})
}

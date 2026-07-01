// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial Phase 2 OGM engine: direct, single-hop UDP exchange
//           between statically configured hub peers.

// Package ogmengine sends and receives OGMs (Originator Messages) over
// UDP, maintaining node_registry/routing_table entries for
// directly-reachable peers. See "Routing Protocol" and "Node/Hub
// Presence and Discovery" in /plan.md.
//
// This is Phase 2 scope only: direct, single-hop OGM exchange between a
// hub and a statically configured list of peer addresses. Multi-hop
// rebroadcast and the BATMAN-adv TQ = EQ/RQ metric are Phase 5.
package ogmengine

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/wire"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// EventHandler is notified of registry-visible effects of received OGMs
// and stale-node sweeps, so callers (e.g. the management API) can turn
// them into wire.ManagementEvent broadcasts without this package needing
// to depend on that one.
type EventHandler interface {
	NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus)
	RouteChanged(route registry.Route)
}

// Config configures an Engine.
type Config struct {
	// SelfID is this hub's own NodeID, sent in every OGM.
	SelfID identity.NodeID
	// BindAddr is the local UDP address to listen on, e.g. "127.0.0.1:9001".
	BindAddr string
	// Peers is the static list of other hubs' UDP addresses to send OGMs
	// to. Automatic LAN discovery is a later phase; Phase 2 hubs are
	// told who their peers are.
	Peers []string
	// Interval is how often an OGM is broadcast to every configured peer.
	Interval time.Duration
	// StaleAfter is how long without a received OGM before a peer is
	// marked stale.
	StaleAfter time.Duration
	// TTL is the OGM's initial hop budget (see "Originator Messages" in
	// /plan.md). Phase 2 hubs only observe it; they never decrement and
	// re-flood, since that is Phase 5 multi-hop behavior.
	TTL uint32

	Registry *registry.Registry
	// Handler is optional; nil disables event notification.
	Handler EventHandler
}

// Engine sends and receives OGMs over UDP for a single hub.
type Engine struct {
	cfg  Config
	conn *net.UDPConn
	seq  uint64 // this hub's own OGM sequence counter

	mu      sync.Mutex
	lastSeq map[string]uint64 // hex NodeID -> last accepted sequence number

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// New creates an Engine. Call Start to open the UDP socket and begin
// sending/receiving.
func New(cfg Config) *Engine {
	return &Engine{
		cfg:     cfg,
		lastSeq: make(map[string]uint64),
	}
}

// Start opens the UDP socket and launches the send/receive/stale-sweep
// goroutines. Call Close to stop them.
func (e *Engine) Start() error {
	addr, err := net.ResolveUDPAddr("udp", e.cfg.BindAddr)
	if err != nil {
		return fmt.Errorf("ogmengine: resolve bind addr %s: %w", e.cfg.BindAddr, err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("ogmengine: listen %s: %w", e.cfg.BindAddr, err)
	}
	e.conn = conn

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.wg.Add(3)
	go e.receiveLoop(ctx)
	go e.sendLoop(ctx)
	go e.staleSweepLoop(ctx)
	return nil
}

// Close stops the engine and releases its UDP socket.
func (e *Engine) Close() error {
	if e.cancel != nil {
		e.cancel()
	}
	var err error
	if e.conn != nil {
		err = e.conn.Close()
	}
	e.wg.Wait()
	return err
}

// LocalAddr returns the engine's bound UDP address, primarily useful in
// tests that bind to port 0 and need to learn the assigned port.
func (e *Engine) LocalAddr() net.Addr {
	return e.conn.LocalAddr()
}

func (e *Engine) sendLoop(ctx context.Context) {
	defer e.wg.Done()
	e.broadcastOnce() // "I'm up": send immediately on start, don't wait a full interval

	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.broadcastOnce()
		}
	}
}

func (e *Engine) broadcastOnce() {
	seq := atomic.AddUint64(&e.seq, 1)
	msg := &wire.Ogm{
		NodeId:            e.cfg.SelfID[:],
		SequenceNumber:    seq,
		Ttl:               e.cfg.TTL,
		IsStartupAnnounce: seq == 1,
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("ogmengine: marshal OGM: %v", err)
		return
	}
	for _, peerAddrStr := range e.cfg.Peers {
		peerAddr, err := net.ResolveUDPAddr("udp", peerAddrStr)
		if err != nil {
			log.Printf("ogmengine: resolve peer %s: %v", peerAddrStr, err)
			continue
		}
		if _, err := e.conn.WriteToUDP(payload, peerAddr); err != nil {
			log.Printf("ogmengine: send to %s: %v", peerAddrStr, err)
		}
	}
}

func (e *Engine) receiveLoop(ctx context.Context) {
	defer e.wg.Done()
	buf := make([]byte, 65535)
	for {
		if ctx.Err() != nil {
			return
		}
		// A short read deadline is how a blocking UDP read is made
		// responsive to ctx cancellation, since net.UDPConn has no
		// context-aware read.
		e.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, _, err := e.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			log.Printf("ogmengine: read: %v", err)
			continue
		}
		e.handlePacket(buf[:n])
	}
}

func (e *Engine) handlePacket(data []byte) {
	var msg wire.Ogm
	if err := proto.Unmarshal(data, &msg); err != nil {
		log.Printf("ogmengine: unmarshal OGM: %v", err)
		return
	}
	if len(msg.NodeId) != len(identity.NodeID{}) {
		return
	}
	var senderID identity.NodeID
	copy(senderID[:], msg.NodeId)

	if senderID == e.cfg.SelfID {
		return // ignore our own broadcasts, e.g. a misconfigured peer list
	}

	key := senderID.String()
	e.mu.Lock()
	last, seen := e.lastSeq[key]
	if seen && msg.SequenceNumber <= last {
		e.mu.Unlock()
		return // replay or out-of-order: drop
	}
	e.lastSeq[key] = msg.SequenceNumber
	e.mu.Unlock()

	statusChanged, err := e.cfg.Registry.UpsertSeen(senderID, time.Now())
	if err != nil {
		log.Printf("ogmengine: UpsertSeen: %v", err)
		return
	}
	if statusChanged && e.cfg.Handler != nil {
		e.cfg.Handler.NodeStatusChanged(senderID, registry.NodeStatusOnline)
	}

	route := registry.Route{
		Destination: senderID,
		NextHop:     senderID,
		TQ:          1.0, // Phase 5 will compute real TQ = EQ/RQ
		LatencyMs:   0,   // not yet measured; Phase 5 adds liveness-ping RTT
		HopCount:    1,   // Phase 2 does not relay/rebroadcast OGMs
	}
	if err := e.cfg.Registry.UpsertRoute(route); err != nil {
		log.Printf("ogmengine: UpsertRoute: %v", err)
		return
	}
	if e.cfg.Handler != nil {
		e.cfg.Handler.RouteChanged(route)
	}
}

func (e *Engine) staleSweepLoop(ctx context.Context) {
	defer e.wg.Done()
	interval := e.cfg.StaleAfter / 2
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stale, err := e.cfg.Registry.MarkStaleBefore(time.Now().Add(-e.cfg.StaleAfter))
			if err != nil {
				log.Printf("ogmengine: MarkStaleBefore: %v", err)
				continue
			}
			if e.cfg.Handler != nil {
				for _, id := range stale {
					e.cfg.Handler.NodeStatusChanged(id, registry.NodeStatusStale)
				}
			}
		}
	}
}

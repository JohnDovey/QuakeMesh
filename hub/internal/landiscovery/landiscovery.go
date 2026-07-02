// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN multicast hub/node discovery on connected Wi-Fi.
//   0.0.19 - lan_context on beacons and segment membership recording.

// Package landiscovery broadcasts hub presence and ingests node beacons on
// the LAN multicast group defined in core/lanbeacon.
package landiscovery

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lanbeacon"
	"github.com/JohnDovey/QuakeMesh/core/lancontext"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
	"github.com/JohnDovey/QuakeMesh/hub/internal/nodeheartbeat"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// Config configures an Engine.
type Config struct {
	BindAddr      string // e.g. "0.0.0.0:47223"; empty disables
	HubNodeID     identity.NodeID
	HeartbeatPort int
	OGMPort       int
	Interval      time.Duration
	Registry      *registry.Registry
	Notifier      nodeheartbeat.Notifier
	Segments      *lansegments.Store
}

// Engine sends hub beacons and registers nodes from LAN announcements.
type Engine struct {
	cfg  Config
	conn *net.UDPConn

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates an Engine. Call Start to begin.
func New(cfg Config) *Engine {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	return &Engine{cfg: cfg}
}

// Start opens the multicast socket and begins beacon loops.
func (e *Engine) Start() error {
	if e.cfg.BindAddr == "" {
		return nil
	}
	addr, err := net.ResolveUDPAddr("udp", e.cfg.BindAddr)
	if err != nil {
		return fmt.Errorf("landiscovery: resolve bind: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("landiscovery: listen %s: %w", e.cfg.BindAddr, err)
	}
	e.conn = conn

	group := net.ParseIP(lanbeacon.MulticastGroup)
	if group == nil {
		conn.Close()
		return fmt.Errorf("landiscovery: invalid multicast group")
	}
	if err := ipv4.NewPacketConn(conn).JoinGroup(nil, &net.UDPAddr{IP: group}); err != nil {
		log.Printf("landiscovery: join multicast group (LAN receive may be limited): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(2)
	go e.sendLoop(ctx)
	go e.receiveLoop(ctx)
	return nil
}

// Close stops beacon loops and closes the socket.
func (e *Engine) Close() error {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	if e.conn != nil {
		return e.conn.Close()
	}
	return nil
}

func (e *Engine) sendLoop(ctx context.Context) {
	defer e.wg.Done()
	e.sendHubBeacon()
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sendHubBeacon()
		}
	}
}

func (e *Engine) sendHubBeacon() {
	var lan *lancontext.Context
	if ctx := lancontext.Detect(); ctx.Valid() {
		lan = &ctx
		if e.cfg.Segments != nil {
			_ = e.cfg.Segments.RecordMembership(lansegments.EntityHub, e.cfg.HubNodeID, ctx, time.Now())
		}
	}
	payload, err := lanbeacon.HubBeacon(
		e.cfg.HubNodeID.String(),
		e.cfg.HeartbeatPort,
		e.cfg.OGMPort,
		lan,
	)
	if err != nil {
		log.Printf("landiscovery: hub beacon: %v", err)
		return
	}
	group := net.ParseIP(lanbeacon.MulticastGroup)
	dst := &net.UDPAddr{IP: group, Port: lanbeacon.MulticastPort}
	if _, err := e.conn.WriteToUDP(payload, dst); err != nil {
		log.Printf("landiscovery: send hub beacon: %v", err)
	}
}

func (e *Engine) receiveLoop(ctx context.Context) {
	defer e.wg.Done()
	buf := make([]byte, 2048)
	for {
		if err := e.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return
		}
		n, _, err := e.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			log.Printf("landiscovery: read: %v", err)
			continue
		}
		msg, ok, err := lanbeacon.Decode(buf[:n])
		if err != nil {
			log.Printf("landiscovery: decode: %v", err)
			continue
		}
		if !ok || msg.Kind != lanbeacon.KindNode {
			continue
		}
		e.handleNodeBeacon(msg)
	}
}

func (e *Engine) handleNodeBeacon(msg lanbeacon.Message) {
	idBytes, err := hex.DecodeString(msg.NodeID)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		return
	}
	var nodeID identity.NodeID
	copy(nodeID[:], idBytes)
	if _, err := nodeheartbeat.RegisterPresence(
		e.cfg.Registry,
		e.cfg.Notifier,
		nodeID,
		msg.Lat,
		msg.Lon,
		time.Now(),
	); err != nil {
		log.Printf("landiscovery: register node %s: %v", msg.NodeID, err)
		return
	}
	if msg.LanContext != nil && e.cfg.Segments != nil {
		_ = e.cfg.Segments.RecordMembership(lansegments.EntityNode, nodeID, *msg.LanContext, time.Now())
	}
}

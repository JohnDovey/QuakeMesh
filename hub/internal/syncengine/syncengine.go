// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9: Hub-to-Hub gossip sync over UDP.

// Package syncengine exchanges HubSyncMessage with configured peer hubs.
package syncengine

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/wire"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// Config configures an Engine.
type Config struct {
	SelfID   identity.NodeID
	BindAddr string
	Peers    []string
	Interval time.Duration
	Registry *registry.Registry
}

// Engine gossips registry state to peer hubs.
type Engine struct {
	cfg  Config
	conn *net.UDPConn

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// New creates an Engine. Call Start to begin gossip.
func New(cfg Config) *Engine {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	return &Engine{cfg: cfg}
}

// Start opens the UDP socket and begins gossip loops.
func (e *Engine) Start() error {
	addr, err := net.ResolveUDPAddr("udp", e.cfg.BindAddr)
	if err != nil {
		return fmt.Errorf("syncengine: resolve bind: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("syncengine: listen %s: %w", e.cfg.BindAddr, err)
	}
	e.conn = conn

	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.wg.Add(2)
	go e.sendLoop(ctx)
	go e.receiveLoop(ctx)
	return nil
}

// Close stops the engine.
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

func (e *Engine) sendLoop(ctx context.Context) {
	defer e.wg.Done()
	e.gossipOnce()
	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.gossipOnce()
		}
	}
}

func (e *Engine) gossipOnce() {
	msg, err := e.buildMessage()
	if err != nil {
		log.Printf("syncengine: build: %v", err)
		return
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	for _, peer := range e.cfg.Peers {
		addr, err := net.ResolveUDPAddr("udp", peer)
		if err != nil {
			continue
		}
		_, _ = e.conn.WriteToUDP(payload, addr)
	}
}

func (e *Engine) buildMessage() (*wire.HubSyncMessage, error) {
	msg := &wire.HubSyncMessage{FromHubId: e.cfg.SelfID[:]}
	nodes, err := e.cfg.Registry.Nodes()
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		rec := &wire.NodeRecord{
			NodeId:           n.NodeID[:],
			FirstSeenUnixMs:  n.FirstSeen.UnixMilli(),
			LastSeenUnixMs:   n.LastSeen.UnixMilli(),
			Status:           string(n.Status),
		}
		if n.LastLat != nil {
			rec.LastLat = *n.LastLat
		}
		if n.LastLon != nil {
			rec.LastLon = *n.LastLon
		}
		msg.NodeRecords = append(msg.NodeRecords, rec)
	}
	relays, err := e.cfg.Registry.ListRelayHubs()
	if err != nil {
		return nil, err
	}
	for _, rh := range relays {
		msg.RelayHubs = append(msg.RelayHubs, &wire.RelayHubRecord{
			HubId:              rh.HubID[:],
			Ip:                 rh.IP,
			Port:               uint32(rh.Port),
			Source:             rh.Source,
			LastVerifiedUnixMs: rh.LastVerified.UnixMilli(),
		})
	}
	return msg, nil
}

func (e *Engine) receiveLoop(ctx context.Context) {
	defer e.wg.Done()
	buf := make([]byte, 65535)
	for {
		if ctx.Err() != nil {
			return
		}
		e.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, _, err := e.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			continue
		}
		var msg wire.HubSyncMessage
		if err := proto.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}
		if len(msg.FromHubId) == len(e.cfg.SelfID) {
			var from identity.NodeID
			copy(from[:], msg.FromHubId)
			if from == e.cfg.SelfID {
				continue
			}
		}
		e.apply(&msg)
	}
}

func (e *Engine) apply(msg *wire.HubSyncMessage) {
	for _, rec := range msg.NodeRecords {
		if len(rec.NodeId) != len(identity.NodeID{}) {
			continue
		}
		var id identity.NodeID
		copy(id[:], rec.NodeId)
		var lat, lon *float64
		if rec.LastLat != 0 || rec.LastLon != 0 {
			lat, lon = &rec.LastLat, &rec.LastLon
		}
		if _, err := e.cfg.Registry.MergeGossipNode(
			id,
			time.UnixMilli(rec.FirstSeenUnixMs),
			time.UnixMilli(rec.LastSeenUnixMs),
			registry.NodeStatus(rec.Status),
			lat, lon,
		); err != nil {
			log.Printf("syncengine: merge node: %v", err)
		}
	}
	for _, rh := range msg.RelayHubs {
		if len(rh.HubId) != len(identity.NodeID{}) || rh.Ip == "" || rh.Port == 0 {
			continue
		}
		var id identity.NodeID
		copy(id[:], rh.HubId)
		if _, err := e.cfg.Registry.UpsertGossipRelay(
			id, rh.Ip, int(rh.Port), time.UnixMilli(rh.LastVerifiedUnixMs),
		); err != nil {
			log.Printf("syncengine: merge relay: %v", err)
		}
	}
}

// PeerSyncAddrs maps OGM peer addresses to sync port (OGM port + 1).
func PeerSyncAddrs(ogmPeers []string) []string {
	out := make([]string, 0, len(ogmPeers))
	for _, peer := range ogmPeers {
		if addr, err := incrementUDPPort(peer); err == nil {
			out = append(out, addr)
		}
	}
	return out
}

// SyncBindAddr returns bind address one UDP port above ogmBind.
func SyncBindAddr(ogmBind string) (string, error) {
	return incrementUDPPort(ogmBind)
}

func incrementUDPPort(hostPort string) (string, error) {
	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return "", err
	}
	addr.Port++
	return addr.String(), nil
}

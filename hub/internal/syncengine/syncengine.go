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

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/banlist"
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
	Apps     *apppresence.Store
	Bans     *banlist.Store
	LocalHub identity.NodeID
	// SignPending is called before each gossip round (e.g. sign Monitor proposals).
	SignPending func()
	// BanNotify receives gossip-merged ban updates for management events.
	BanNotify interface {
		BanProposalChanged(banID [16]byte, appID, versionRange string)
		BanVerdictChanged(banID [16]byte, hubID identity.NodeID, agree bool)
	}
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
	if e.cfg.SignPending != nil {
		e.cfg.SignPending()
	}
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
	if e.cfg.Apps != nil {
		apps, err := e.cfg.Apps.List()
		if err != nil {
			return nil, err
		}
		for _, a := range apps {
			if e.cfg.Bans != nil {
				blocked, err := e.cfg.Bans.IsLocallyEnforced(e.cfg.LocalHub, a.AppID, a.AppVersion)
				if err == nil && blocked {
					continue
				}
			}
			msg.AppPresence = append(msg.AppPresence, &wire.AppPresenceRecord{
				NodeId:             a.NodeID[:],
				AppId:              a.AppID,
				AppName:            a.AppName,
				AppVersion:         a.AppVersion,
				LastReportedUnixMs: a.LastReported.UnixMilli(),
			})
		}
	}
	if e.cfg.Bans != nil {
		proposals, err := e.cfg.Bans.ListProposals()
		if err != nil {
			return nil, err
		}
		for _, p := range proposals {
			msg.BanProposals = append(msg.BanProposals, &wire.BanProposal{
				BanId:              p.BanID[:],
				AppId:              p.AppID,
				VersionRange:       p.VersionRange,
				Reason:             p.Reason,
				ProposedByHubId:    p.ProposedBy[:],
				ProposedAtUnixMs:   p.ProposedAt.UnixMilli(),
				Signature:          p.Signature,
			})
		}
		verdicts, err := e.cfg.Bans.ListAllVerdicts()
		if err != nil {
			return nil, err
		}
		for _, v := range verdicts {
			msg.BanVerdicts = append(msg.BanVerdicts, &wire.BanVerdict{
				BanId:           v.BanID[:],
				HubId:           v.HubID[:],
				Agree:           v.Agree,
				DecidedAtUnixMs: v.DecidedAt.UnixMilli(),
			})
		}
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
	if e.cfg.Apps != nil {
		for _, ap := range msg.AppPresence {
			if len(ap.NodeId) != len(identity.NodeID{}) || ap.AppId == "" {
				continue
			}
			var id identity.NodeID
			copy(id[:], ap.NodeId)
			if _, err := e.cfg.Apps.MergeGossip(
				id, ap.AppId, ap.AppName, ap.AppVersion,
				time.UnixMilli(ap.LastReportedUnixMs),
			); err != nil {
				log.Printf("syncengine: merge app presence: %v", err)
			}
		}
	}
	if e.cfg.Bans != nil {
		for _, bp := range msg.BanProposals {
			if len(bp.BanId) != 16 {
				continue
			}
			var banID [16]byte
			copy(banID[:], bp.BanId)
			var proposer identity.NodeID
			copy(proposer[:], bp.ProposedByHubId)
			p := banlist.Proposal{
				BanID: banID, AppID: bp.AppId, VersionRange: bp.VersionRange,
				Reason: bp.Reason, ProposedBy: proposer,
				ProposedAt: time.UnixMilli(bp.ProposedAtUnixMs), Signature: bp.Signature,
			}
			if changed, err := e.cfg.Bans.MergeGossipProposal(p); err != nil {
				log.Printf("syncengine: merge ban proposal: %v", err)
			} else if changed && e.cfg.BanNotify != nil {
				e.cfg.BanNotify.BanProposalChanged(banID, bp.AppId, bp.VersionRange)
			}
		}
		for _, bv := range msg.BanVerdicts {
			if len(bv.BanId) != 16 || len(bv.HubId) != len(identity.NodeID{}) {
				continue
			}
			var banID [16]byte
			copy(banID[:], bv.BanId)
			var hubID identity.NodeID
			copy(hubID[:], bv.HubId)
			if changed, err := e.cfg.Bans.MergeGossipVerdict(banlist.Verdict{
				BanID: banID, HubID: hubID, Agree: bv.Agree,
				DecidedAt: time.UnixMilli(bv.DecidedAtUnixMs),
			}); err != nil {
				log.Printf("syncengine: merge ban verdict: %v", err)
			} else if changed && e.cfg.BanNotify != nil {
				e.cfg.BanNotify.BanVerdictChanged(banID, hubID, bv.Agree)
			}
		}
	}
}

// PeerSyncAddrs maps OGM peer addresses to the gossip sync port (OGM port + 3).
func PeerSyncAddrs(ogmPeers []string) []string {
	out := make([]string, 0, len(ogmPeers))
	for _, peer := range ogmPeers {
		addr, err := net.ResolveUDPAddr("udp", peer)
		if err != nil {
			continue
		}
		addr.Port += 3
		out = append(out, addr.String())
	}
	return out
}

// SyncBindAddr returns the UDP bind address for hub-to-hub gossip (OGM port + 3).
// Port OGM+1 (47223) is reserved for LAN multicast discovery beacons.
func SyncBindAddr(ogmBind string) (string, error) {
	addr, err := net.ResolveUDPAddr("udp", ogmBind)
	if err != nil {
		return "", err
	}
	addr.Port += 3
	return addr.String(), nil
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial Phase 2 OGM engine: direct, single-hop UDP exchange
//           between statically configured hub peers.
//   0.0.6 - Phase 5: multi-hop OGM rebroadcast, TQ = EQ/RQ window, Hello
//           RTT measurement, route metric selection, and failover on stale
//           next-hops.
//   0.0.7 - Phase 6: stale-node probing, revival "I'm up" rebroadcast.
//   0.0.7 - Register direct peer hubs in hub_registry; hub stale sweep.
//   0.0.9 - Record hub-observed proximity events for trust scoring.
//   0.0.10 - Phase 8: apply GPS coordinates from received OGMs.

// Package ogmengine sends and receives OGMs (Originator Messages) over
// UDP, maintaining node_registry/routing_table entries for reachable
// nodes. See "Routing Protocol" and "Node/Hub Presence and Discovery"
// in /plan.md.
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
	"github.com/JohnDovey/QuakeMesh/core/routing"
	"github.com/JohnDovey/QuakeMesh/core/trust"
	"github.com/JohnDovey/QuakeMesh/core/wire"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// EventHandler is notified of registry-visible effects of received OGMs
// and stale-node sweeps, so callers (e.g. the management API) can turn
// them into wire.ManagementEvent broadcasts without this package needing
// to depend on that one.
type EventHandler interface {
	NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus)
	HubStatusChanged(hubID identity.NodeID, status registry.HubStatus)
	RouteChanged(route registry.Route)
}

// Config configures an Engine.
type Config struct {
	SelfID     identity.NodeID
	BindAddr   string
	Peers      []string
	Interval   time.Duration
	StaleAfter time.Duration
	TTL        uint32
	Registry   *registry.Registry
	Trust      *trust.Store
	Handler    EventHandler
}

// Engine sends and receives OGMs over UDP for a single hub.
type Engine struct {
	cfg  Config
	conn *net.UDPConn
	seq  uint64

	mu            sync.Mutex
	lastSeq       map[string]uint64
	peerByAddr    map[string]identity.NodeID
	rebroadcasted map[string]uint64
	linkRQ        map[string]uint32
	linkEQ        map[string]uint32
	latencyMs     map[string]int
	alternates    map[string][]registry.Route
	pendingHello  map[string]time.Time

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// New creates an Engine. Call Start to open the UDP socket and begin
// sending/receiving.
func New(cfg Config) *Engine {
	return &Engine{
		cfg:           cfg,
		lastSeq:       make(map[string]uint64),
		peerByAddr:    make(map[string]identity.NodeID),
		rebroadcasted: make(map[string]uint64),
		linkRQ:        make(map[string]uint32),
		linkEQ:        make(map[string]uint32),
		latencyMs:     make(map[string]int),
		alternates:    make(map[string][]registry.Route),
		pendingHello:  make(map[string]time.Time),
	}
}

// Start opens the UDP socket and launches worker goroutines.
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

	e.wg.Add(5)
	go e.receiveLoop(ctx)
	go e.sendLoop(ctx)
	go e.staleSweepLoop(ctx)
	go e.helloLoop(ctx)
	go e.staleProbeLoop(ctx)
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

// LocalAddr returns the engine's bound UDP address.
func (e *Engine) LocalAddr() net.Addr {
	return e.conn.LocalAddr()
}

func (e *Engine) sendLoop(ctx context.Context) {
	defer e.wg.Done()
	e.broadcastOnce()

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
		HopCount:          0,
		IsStartupAnnounce: seq == 1,
	}
	e.floodOgm(msg, "")
}

func (e *Engine) broadcastStartup() {
	seq := atomic.AddUint64(&e.seq, 1)
	msg := &wire.Ogm{
		NodeId:            e.cfg.SelfID[:],
		SequenceNumber:    seq,
		Ttl:               e.cfg.TTL,
		HopCount:          0,
		IsStartupAnnounce: true,
	}
	e.floodOgm(msg, "")
}

func (e *Engine) rebroadcastStartup(msg *wire.Ogm, fromAddr string) {
	if msg.Ttl == 0 {
		return
	}
	reb := proto.Clone(msg).(*wire.Ogm)
	reb.IsStartupAnnounce = true
	reb.Ttl--
	reb.HopCount++
	reb.LastHopId = e.cfg.SelfID[:]
	e.floodOgm(reb, fromAddr)
}

func (e *Engine) floodOgm(msg *wire.Ogm, exceptAddr string) {
	payload, err := proto.Marshal(msg)
	if err != nil {
		log.Printf("ogmengine: marshal OGM: %v", err)
		return
	}
	for _, peerAddrStr := range e.cfg.Peers {
		if peerAddrStr == exceptAddr {
			continue
		}
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
		e.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, raddr, err := e.conn.ReadFromUDP(buf)
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
		e.handlePacket(buf[:n], raddr)
	}
}

func (e *Engine) handlePacket(data []byte, raddr *net.UDPAddr) {
	var ogm wire.Ogm
	if err := proto.Unmarshal(data, &ogm); err == nil && len(ogm.NodeId) == len(identity.NodeID{}) {
		e.handleOgm(&ogm, raddr)
		return
	}
	var hello wire.Hello
	if err := proto.Unmarshal(data, &hello); err == nil && len(hello.NodeId) == len(identity.NodeID{}) {
		e.handleHello(&hello, raddr)
	}
}

func (e *Engine) handleHello(msg *wire.Hello, raddr *net.UDPAddr) {
	var senderID identity.NodeID
	copy(senderID[:], msg.NodeId)
	e.mu.Lock()
	if sentAt, ok := e.pendingHello[raddr.String()]; ok {
		rtt := time.Since(sentAt)
		if rtt > 0 && rtt < 10*time.Second {
			e.latencyMs[senderID.String()] = int(rtt.Milliseconds())
		}
		delete(e.pendingHello, raddr.String())
	}
	e.mu.Unlock()
}

func (e *Engine) handleOgm(msg *wire.Ogm, raddr *net.UDPAddr) {
	var originID identity.NodeID
	copy(originID[:], msg.NodeId)
	if originID == e.cfg.SelfID {
		if msg.HopCount > 0 {
			forwarder := e.resolveForwarder(msg, raddr)
			e.mu.Lock()
			e.linkEQ[forwarder.String()]++
			e.mu.Unlock()
		}
		return
	}

	forwarder := e.resolveForwarder(msg, raddr)
	addrKey := raddr.String()

	e.mu.Lock()
	if msg.HopCount == 0 && len(msg.LastHopId) == 0 {
		e.peerByAddr[addrKey] = originID
		forwarder = originID
	}
	e.linkRQ[forwarder.String()]++
	originKey := originID.String()
	last, seen := e.lastSeq[originKey]
	if seen && msg.SequenceNumber <= last {
		e.mu.Unlock()
		return
	}
	e.lastSeq[originKey] = msg.SequenceNumber
	e.mu.Unlock()

	if msg.HopCount == 0 && originID != e.cfg.SelfID {
		e.upsertPeerHub(originID, raddr)
	}

	statusChanged, err := e.cfg.Registry.UpsertSeen(originID, time.Now())
	if err != nil {
		log.Printf("ogmengine: UpsertSeen: %v", err)
		return
	}
	if statusChanged && e.cfg.Handler != nil {
		e.cfg.Handler.NodeStatusChanged(originID, registry.NodeStatusOnline)
		e.rebroadcastStartup(msg, addrKey)
	}

	e.recordProximity(originID, msg.HopCount)
	e.applyLocation(originID, msg)

	hopCount := int(msg.HopCount) + 1
	linkTQ := e.linkTQ(forwarder)
	pathTQ := linkTQ
	if hopCount > 1 {
		pathTQ = routing.PathTQ(linkTQ, routing.TransmitQuality(0.85))
	}
	latency := e.neighborLatency(forwarder)

	candidate := registry.Route{
		Destination: originID,
		NextHop:     forwarder,
		TQ:          float64(pathTQ),
		LatencyMs:   latency * hopCount,
		HopCount:    hopCount,
	}
	e.maybeUpsertRoute(candidate)

	if msg.Ttl > 0 {
		e.maybeRebroadcast(msg, addrKey)
	}
}

func (e *Engine) resolveForwarder(msg *wire.Ogm, raddr *net.UDPAddr) identity.NodeID {
	if len(msg.LastHopId) == len(identity.NodeID{}) {
		var id identity.NodeID
		copy(id[:], msg.LastHopId)
		return id
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if id, ok := e.peerByAddr[raddr.String()]; ok {
		return id
	}
	var originID identity.NodeID
	copy(originID[:], msg.NodeId)
	return originID
}

func (e *Engine) linkTQ(neighbor identity.NodeID) routing.TransmitQuality {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := neighbor.String()
	return routing.ComputeTQ(e.linkEQ[key], maxUint32(e.linkRQ[key], 1))
}

func (e *Engine) neighborLatency(neighbor identity.NodeID) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if ms, ok := e.latencyMs[neighbor.String()]; ok {
		return ms
	}
	return 0
}

func (e *Engine) maybeUpsertRoute(candidate registry.Route) {
	current, has, err := e.cfg.Registry.GetRoute(candidate.Destination)
	if err != nil {
		log.Printf("ogmengine: GetRoute: %v", err)
		return
	}

	cur := routing.Route{
		DestinationNodeID: candidate.Destination,
		NextHopNodeID:     candidate.NextHop,
		TQ:                routing.TransmitQuality(candidate.TQ),
		LatencyMillis:     uint32(candidate.LatencyMs),
		HopCount:          uint32(candidate.HopCount),
	}
	var prev routing.Route
	if has {
		prev = routing.Route{
			DestinationNodeID: current.Destination,
			NextHopNodeID:     current.NextHop,
			TQ:                routing.TransmitQuality(current.TQ),
			LatencyMillis:     uint32(current.LatencyMs),
			HopCount:          uint32(current.HopCount),
		}
	}
	if has && !routing.Better(cur, prev) {
		e.storeAlternate(candidate)
		return
	}
	if has {
		e.storeAlternate(current)
	}
	if err := e.cfg.Registry.UpsertRoute(candidate); err != nil {
		log.Printf("ogmengine: UpsertRoute: %v", err)
		return
	}
	if e.cfg.Handler != nil {
		e.cfg.Handler.RouteChanged(candidate)
	}
}

func (e *Engine) storeAlternate(route registry.Route) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := route.Destination.String()
	alts := e.alternates[key]
	for _, existing := range alts {
		if existing.NextHop == route.NextHop {
			return
		}
	}
	e.alternates[key] = append(alts, route)
	if len(e.alternates[key]) > 3 {
		e.alternates[key] = e.alternates[key][len(e.alternates[key])-3:]
	}
}

func (e *Engine) maybeRebroadcast(msg *wire.Ogm, fromAddr string) {
	var originID identity.NodeID
	copy(originID[:], msg.NodeId)
	originKey := originID.String()
	e.mu.Lock()
	if msg.SequenceNumber <= e.rebroadcasted[originKey] {
		e.mu.Unlock()
		return
	}
	e.rebroadcasted[originKey] = msg.SequenceNumber
	e.mu.Unlock()

	reb := proto.Clone(msg).(*wire.Ogm)
	reb.Ttl--
	reb.HopCount++
	reb.LastHopId = e.cfg.SelfID[:]
	e.floodOgm(reb, fromAddr)
}

func (e *Engine) helloLoop(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sendHellos()
		}
	}
}

func (e *Engine) sendHellos() {
	msg := &wire.Hello{
		NodeId:         e.cfg.SelfID[:],
		SentAtUnixMs:   time.Now().UnixMilli(),
	}
	payload, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	now := time.Now()
	for _, peerAddrStr := range e.cfg.Peers {
		peerAddr, err := net.ResolveUDPAddr("udp", peerAddrStr)
		if err != nil {
			continue
		}
		e.mu.Lock()
		e.pendingHello[peerAddrStr] = now
		e.mu.Unlock()
		_, _ = e.conn.WriteToUDP(payload, peerAddr)
	}
}

func (e *Engine) staleProbeLoop(ctx context.Context) {
	defer e.wg.Done()
	interval := e.cfg.StaleAfter / 3
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
			stale, err := e.staleNodeIDs()
			if err != nil || len(stale) == 0 {
				continue
			}
			e.sendHellos()
			e.broadcastStartup()
		}
	}
}

func (e *Engine) staleNodeIDs() ([]identity.NodeID, error) {
	nodes, err := e.cfg.Registry.Nodes()
	if err != nil {
		return nil, err
	}
	var stale []identity.NodeID
	for _, n := range nodes {
		if n.Status == registry.NodeStatusStale {
			stale = append(stale, n.NodeID)
		}
	}
	return stale, nil
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
			for _, id := range stale {
				e.failoverRoutes(id)
				if e.cfg.Handler != nil {
					e.cfg.Handler.NodeStatusChanged(id, registry.NodeStatusStale)
				}
			}
			staleHubs, err := e.cfg.Registry.MarkHubsStaleBefore(time.Now().Add(-e.cfg.StaleAfter))
			if err != nil {
				log.Printf("ogmengine: MarkHubsStaleBefore: %v", err)
				continue
			}
			for _, id := range staleHubs {
				if e.cfg.Handler != nil {
					e.cfg.Handler.HubStatusChanged(id, registry.HubStatusStale)
				}
			}
		}
	}
}

func (e *Engine) upsertPeerHub(hubID identity.NodeID, raddr *net.UDPAddr) {
	changed, err := e.cfg.Registry.UpsertHubSeen(hubID, raddr.IP.String(), raddr.Port, time.Now())
	if err != nil {
		log.Printf("ogmengine: UpsertHubSeen: %v", err)
		return
	}
	if changed && e.cfg.Handler != nil {
		e.cfg.Handler.HubStatusChanged(hubID, registry.HubStatusOnline)
	}
}

func (e *Engine) recordProximity(observed identity.NodeID, hopCount uint32) {
	if e.cfg.Trust == nil {
		return
	}
	transport := trust.TransportHubRelay
	rssi := -80
	if hopCount == 0 {
		transport = trust.TransportHubDirect
		rssi = -55
	}
	if err := e.cfg.Trust.RecordProximity(e.cfg.SelfID, observed, rssi, transport, time.Now()); err != nil {
		log.Printf("ogmengine: RecordProximity: %v", err)
	}
}

func (e *Engine) applyLocation(nodeID identity.NodeID, msg *wire.Ogm) {
	if msg.Lat == nil || msg.Lon == nil {
		return
	}
	lat, lon := msg.GetLat(), msg.GetLon()
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return
	}
	if lat == 0 && lon == 0 {
		return
	}
	at := time.Now()
	if msg.LocationAtUnixMs != nil && msg.GetLocationAtUnixMs() > 0 {
		at = time.UnixMilli(msg.GetLocationAtUnixMs())
	}
	if err := e.cfg.Registry.UpdateLocation(nodeID, lat, lon, at); err != nil {
		log.Printf("ogmengine: UpdateLocation: %v", err)
	}
}

func (e *Engine) failoverRoutes(staleNode identity.NodeID) {
	if _, err := e.cfg.Registry.DeleteRoutesViaNextHop(staleNode); err != nil {
		log.Printf("ogmengine: DeleteRoutesViaNextHop: %v", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for destKey, alts := range e.alternates {
		var remaining []registry.Route
		var promoted *registry.Route
		for _, alt := range alts {
			if alt.NextHop == staleNode {
				continue
			}
			remaining = append(remaining, alt)
			if promoted == nil {
				copy := alt
				promoted = &copy
			}
		}
		if promoted != nil {
			_ = e.cfg.Registry.UpsertRoute(*promoted)
			if e.cfg.Handler != nil {
				e.cfg.Handler.RouteChanged(*promoted)
			}
		}
		e.alternates[destKey] = remaining
	}
}

func maxUint32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

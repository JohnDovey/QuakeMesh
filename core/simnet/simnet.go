// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.2 - Implemented the virtual node fabric: simulated loss/latency,
//           peer-up/peer-down notifications (with join-time replay for
//           the joining node's own view), transport.Transport wiring.

// Package simnet provides a virtual simulated-network test harness: N
// in-process nodes over a simulated lossy/latent transport, for testing
// routing and trust convergence without real radios. See "Phase 1" and
// "Verification Strategy" in /plan.md.
package simnet

import (
	"math/rand"
	"sync"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/transport"
)

// Config controls the simulated link characteristics of a Fabric.
type Config struct {
	// PacketLoss is the probability, in [0,1], that any given sent frame
	// is dropped in transit.
	PacketLoss float64
	// Latency is the simulated one-way delivery delay applied to every
	// frame that is not dropped.
	Latency time.Duration
	// Rand supplies randomness for loss simulation. If nil, a
	// time-seeded source is used; set your own for reproducible tests.
	Rand *rand.Rand
}

// Fabric is an in-process simulated network connecting VirtualNodes,
// standing in for real radios (BLE/Wi-Fi Direct/LAN) in tests.
type Fabric struct {
	cfg Config

	mu    sync.Mutex
	nodes map[string]*VirtualNode
}

// NewFabric creates a Fabric with the given simulated link characteristics.
func NewFabric(cfg Config) *Fabric {
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Fabric{cfg: cfg, nodes: make(map[string]*VirtualNode)}
}

// NewNode registers a new virtual node with the given id on the fabric.
// Every already-registered node is notified of the new peer via
// OnPeerUp. The new node itself learns about already-present peers when
// it registers its own OnPeerUp handler (see VirtualNode.OnPeerUp) --
// registration happens after NewNode returns, so it cannot be notified
// synchronously here.
func (f *Fabric) NewNode(id transport.PeerID) *VirtualNode {
	f.mu.Lock()
	n := &VirtualNode{id: id, fabric: f}
	key := string(id)
	f.nodes[key] = n
	existingPeers := f.snapshotPeersLocked(key)
	f.mu.Unlock()

	for _, peer := range existingPeers {
		peer.notifyPeerUp(id)
	}
	return n
}

// RemoveNode removes the node with the given id from the fabric,
// notifying every remaining peer via OnPeerDown -- simulating a node
// going out of range or powering off mid-test.
func (f *Fabric) RemoveNode(id transport.PeerID) {
	f.mu.Lock()
	delete(f.nodes, string(id))
	remaining := f.snapshotPeersLocked(string(id))
	f.mu.Unlock()

	for _, peer := range remaining {
		peer.notifyPeerDown(id)
	}
}

// snapshotPeersLocked returns every registered node except the one keyed
// by except. Caller must hold f.mu; the returned slice is safe to use
// after unlocking.
func (f *Fabric) snapshotPeersLocked(except string) []*VirtualNode {
	peers := make([]*VirtualNode, 0, len(f.nodes))
	for k, n := range f.nodes {
		if k == except {
			continue
		}
		peers = append(peers, n)
	}
	return peers
}

// peerIDsExcept returns the ids of every node currently on the fabric
// except id.
func (f *Fabric) peerIDsExcept(id transport.PeerID) []transport.PeerID {
	f.mu.Lock()
	peers := f.snapshotPeersLocked(string(id))
	f.mu.Unlock()

	ids := make([]transport.PeerID, len(peers))
	for i, p := range peers {
		ids[i] = p.id
	}
	return ids
}

func (f *Fabric) deliver(from, to transport.PeerID, frame []byte) {
	f.mu.Lock()
	dest, ok := f.nodes[string(to)]
	loss, latency, rng := f.cfg.PacketLoss, f.cfg.Latency, f.cfg.Rand
	f.mu.Unlock()

	if !ok {
		return // peer not on the fabric: analogous to out of range
	}
	if loss > 0 && rng.Float64() < loss {
		return // simulated drop
	}

	deliver := func() { dest.receive(from, frame) }
	if latency > 0 {
		time.AfterFunc(latency, deliver)
	} else {
		go deliver()
	}
}

// VirtualNode is one in-process simulated participant. It implements
// transport.Transport so mesh-core routing/trust logic under test never
// needs to know it isn't a real radio link.
type VirtualNode struct {
	id     transport.PeerID
	fabric *Fabric

	mu         sync.RWMutex
	onReceive  func(peer transport.PeerID, frame []byte)
	onPeerUp   func(peer transport.PeerID)
	onPeerDown func(peer transport.PeerID)
}

var _ transport.Transport = (*VirtualNode)(nil)

// ID returns this virtual node's peer id on the fabric.
func (n *VirtualNode) ID() transport.PeerID {
	return n.id
}

// Send delivers frame to peer through the fabric, subject to the
// fabric's configured simulated loss and latency.
func (n *VirtualNode) Send(peer transport.PeerID, frame []byte) error {
	n.fabric.deliver(n.id, peer, frame)
	return nil
}

// OnReceive registers the handler invoked when a frame arrives for this
// node. Replaces any previously registered handler.
func (n *VirtualNode) OnReceive(handler func(peer transport.PeerID, frame []byte)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onReceive = handler
}

// OnPeerUp registers the handler invoked when another node joins the
// fabric, replacing any previously registered handler, and immediately
// replays every peer already on the fabric so callers don't have to
// separately ask "who's already here" -- joining is indistinguishable
// from being told about everyone one at a time.
func (n *VirtualNode) OnPeerUp(handler func(peer transport.PeerID)) {
	n.mu.Lock()
	n.onPeerUp = handler
	n.mu.Unlock()

	if handler == nil {
		return
	}
	for _, peer := range n.fabric.peerIDsExcept(n.id) {
		handler(peer)
	}
}

// OnPeerDown registers the handler invoked when another node leaves the
// fabric. Replaces any previously registered handler.
func (n *VirtualNode) OnPeerDown(handler func(peer transport.PeerID)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onPeerDown = handler
}

func (n *VirtualNode) receive(from transport.PeerID, frame []byte) {
	n.mu.RLock()
	h := n.onReceive
	n.mu.RUnlock()
	if h != nil {
		h(from, frame)
	}
}

func (n *VirtualNode) notifyPeerUp(peer transport.PeerID) {
	n.mu.RLock()
	h := n.onPeerUp
	n.mu.RUnlock()
	if h != nil {
		h(peer)
	}
}

func (n *VirtualNode) notifyPeerDown(peer transport.PeerID) {
	n.mu.RLock()
	h := n.onPeerDown
	n.mu.RUnlock()
	if h != nil {
		h(peer)
	}
}

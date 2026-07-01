// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: gomobile-bindable Node facade wrapping /core
//           identity and SQLite for the Android app.

// Package mobile is the gomobile-bindable surface of QuakeMesh core for
// Android. Kotlin transport shims deliver inbound frames via
// OnFrameReceived and receive outbound frames through FrameSink.
//
// Bind with:
//
//	gomobile bind -target=android -o meshcore.aar ./mobile
//
// See /android/gomobile-bind.sh and Phase 4 in /plan.md.
package mobile

import (
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// FrameSink is implemented in Kotlin. The Go core calls SendFrame when a
// transport should deliver an outbound mesh frame to peerHex (hex NodeID
// or transport-scoped peer address agreed by both sides).
type FrameSink interface {
	SendFrame(peerHex string, frame []byte)
}

// Node is the Android-facing mesh core instance.
type Node struct {
	mu       sync.Mutex
	identity *identity.Identity
	db       *storage.DB
	sink     FrameSink
}

// NewNode loads or creates identity at identityPath and opens (or creates)
// the Android registry database at dbPath.
func NewNode(identityPath, dbPath string) (*Node, error) {
	id, err := identity.LoadOrCreate(identityPath)
	if err != nil {
		return nil, fmt.Errorf("mobile: identity: %w", err)
	}
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("mobile: storage: %w", err)
	}
	return &Node{identity: id, db: db}, nil
}

// NodeID returns this node's mesh identity as lowercase hex.
func (n *Node) NodeID() string {
	return n.identity.NodeID.String()
}

// SetFrameSink registers the Kotlin transport bridge for outbound frames.
func (n *Node) SetFrameSink(sink FrameSink) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}

// OnFrameReceived is called by Kotlin transports when an inbound frame
// arrives from peerHex. Phase 4 records receipt; routing is wired in
// later phases.
func (n *Node) OnFrameReceived(peerHex string, frame []byte) error {
	if _, err := hex.DecodeString(peerHex); err != nil && len(peerHex) > 0 {
		return fmt.Errorf("mobile: invalid peer id %q: %w", peerHex, err)
	}
	_ = frame
	return nil
}

// EmitFrame sends a frame out via the registered FrameSink.
func (n *Node) EmitFrame(peerHex string, frame []byte) error {
	n.mu.Lock()
	sink := n.sink
	n.mu.Unlock()
	if sink == nil {
		return fmt.Errorf("mobile: no FrameSink registered")
	}
	sink.SendFrame(peerHex, frame)
	return nil
}

// Close releases the registry database.
func (n *Node) Close() error {
	return n.db.Close()
}

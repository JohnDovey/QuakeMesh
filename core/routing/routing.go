// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package routing will implement BATMAN-adv-inspired OGM routing: per-hop
// bidirectional link quality (TQ = EQ/RQ) and a next-hop metric weighing
// TQ, latency, and loss rate. See "Routing Protocol" in /plan.md.
//
// Not yet implemented (Phase 1/5).
package routing

// TransmitQuality is a link's TQ = EQ/RQ, in [0.0, 1.0].
type TransmitQuality float64

// Route is the best known next hop toward a destination.
type Route struct {
	DestinationNodeID [32]byte
	NextHopNodeID     [32]byte
	TQ                TransmitQuality
	LatencyMillis     uint32
	HopCount          uint32
}

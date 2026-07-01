// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package simnet will provide a virtual simulated-network test harness:
// N in-process nodes over a simulated lossy/latent transport, for testing
// routing and trust convergence without real radios. See "Phase 1" and
// "Verification Strategy" in /plan.md.
//
// Not yet implemented (Phase 1).
package simnet

// VirtualNode is one in-process simulated participant.
type VirtualNode struct {
	NodeID [32]byte
}

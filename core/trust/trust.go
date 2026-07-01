// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package trust will implement the 0-100 per-node trust score: longevity,
// physical proximity history, and diminishing-returns explicit
// endorsements. See "Trust Register" in /plan.md.
//
// Not yet implemented (Phase 7).
package trust

// Score is a node's trust score in [0, 100].
type Score int

// Breakdown is the per-component contribution to a node's Score.
type Breakdown struct {
	NodeID               [32]byte
	LongevityComponent   Score
	ProximityComponent   Score
	EndorsementComponent Score
	Total                Score
}

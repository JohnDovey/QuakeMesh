// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.9 - Phase 7: trust score computation and SQLite store.

// Package trust implements the 0-100 per-node trust score: longevity,
// physical proximity history, and diminishing-returns explicit
// endorsements. See "Trust Register" in /plan.md.
package trust

import "github.com/JohnDovey/QuakeMesh/core/identity"

// Score is a node's trust score in [0, 100].
type Score int

// Breakdown is the per-component contribution to a node's Score.
type Breakdown struct {
	NodeID               identity.NodeID
	LongevityComponent   Score
	ProximityComponent   Score
	EndorsementComponent Score
	Total                Score
}

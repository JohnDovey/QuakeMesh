// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.6 - Phase 5: TQ computation, route cost metric, and best-route
//           selection weighing TQ, latency, and hop count.

// Package routing implements BATMAN-adv-inspired route selection. See
// "Routing Protocol" in /plan.md.
package routing

import "math"

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

// ComputeTQ returns EQ/RQ clamped to [0, 1]. If rq is zero, returns 0.
func ComputeTQ(eq, rq uint32) TransmitQuality {
	if rq == 0 {
		return 0
	}
	tq := float64(eq) / float64(rq)
	if tq > 1 {
		tq = 1
	}
	if tq < 0 {
		tq = 0
	}
	return TransmitQuality(tq)
}

// Cost returns a lower-is-better score combining hop count, latency, and
// TQ per "Routing Metric" in /plan.md.
func Cost(r Route) float64 {
	hopPenalty := float64(r.HopCount) * 10.0
	latencyPenalty := float64(r.LatencyMillis) * 0.05
	tqPenalty := (1.0 - float64(r.TQ)) * 40.0
	return hopPenalty + latencyPenalty + tqPenalty
}

// Better reports whether candidate is strictly preferable to current.
// A zero destination in current means no route yet.
func Better(candidate, current Route) bool {
	if current.DestinationNodeID == [32]byte{} {
		return true
	}
	c1, c2 := Cost(candidate), Cost(current)
	if math.Abs(c1-c2) > 0.001 {
		return c1 < c2
	}
	if candidate.TQ != current.TQ {
		return candidate.TQ > current.TQ
	}
	return candidate.HopCount < current.HopCount
}

// PathTQ degrades propagated TQ across hops (cumulative link reliability).
func PathTQ(linkTQ, viaTQ TransmitQuality) TransmitQuality {
	v := float64(linkTQ) * float64(viaTQ)
	if v > 1 {
		return 1
	}
	return TransmitQuality(v)
}

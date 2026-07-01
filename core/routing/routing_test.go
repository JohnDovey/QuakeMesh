// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.6 - Phase 5 routing metric tests.

package routing

import "testing"

func TestComputeTQ(t *testing.T) {
	if got := ComputeTQ(8, 10); got < 0.79 || got > 0.81 {
		t.Fatalf("ComputeTQ(8,10) = %v, want ~0.8", got)
	}
	if ComputeTQ(1, 0) != 0 {
		t.Fatal("rq=0 should yield 0")
	}
}

func TestBetter_prefersLowerHopWhenTQEqual(t *testing.T) {
	dest := [32]byte{1}
	short := Route{DestinationNodeID: dest, NextHopNodeID: [32]byte{2}, TQ: 1, HopCount: 1}
	long := Route{DestinationNodeID: dest, NextHopNodeID: [32]byte{3}, TQ: 1, HopCount: 3}
	if !Better(short, long) {
		t.Fatal("expected shorter hop route to win")
	}
}

func TestBetter_prefersHigherTQOnSimilarHops(t *testing.T) {
	dest := [32]byte{1}
	good := Route{DestinationNodeID: dest, NextHopNodeID: [32]byte{2}, TQ: 0.9, HopCount: 2, LatencyMillis: 10}
	poor := Route{DestinationNodeID: dest, NextHopNodeID: [32]byte{3}, TQ: 0.4, HopCount: 2, LatencyMillis: 10}
	if !Better(good, poor) {
		t.Fatal("expected higher TQ route to win")
	}
}

func TestPathTQ(t *testing.T) {
	got := PathTQ(0.8, 0.5)
	if got < 0.39 || got > 0.41 {
		t.Fatalf("PathTQ = %v, want 0.4", got)
	}
}

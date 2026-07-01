// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.9 - Phase 7 trust score tests.

package trust

import (
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
)

func TestCompute_LongevitySaturates(t *testing.T) {
	now := time.Now()
	b := Compute(identity.NodeID{}, Metrics{
		FirstSeen: now.Add(-24 * time.Hour),
		Now:       now,
	})
	if b.LongevityComponent <= 0 {
		t.Fatalf("expected positive longevity for 1-day node, got %d", b.LongevityComponent)
	}
	bOld := Compute(identity.NodeID{}, Metrics{
		FirstSeen: now.Add(-180 * 24 * time.Hour),
		Now:       now,
	})
	if bOld.LongevityComponent < maxLongevity-2 {
		t.Fatalf("expected near-max longevity after 180d, got %d", bOld.LongevityComponent)
	}
}

func TestCompute_EndorsementDiminishingReturns(t *testing.T) {
	now := time.Now()
	first := now.Add(-time.Hour)
	one := Compute(identity.NodeID{}, Metrics{FirstSeen: first, Now: now, UniqueEndorsers: 1})
	many := Compute(identity.NodeID{}, Metrics{FirstSeen: first, Now: now, UniqueEndorsers: 100})
	if many.EndorsementComponent <= one.EndorsementComponent {
		t.Fatalf("100 endorsers should beat 1: %d vs %d", many.EndorsementComponent, one.EndorsementComponent)
	}
	if many.EndorsementComponent > maxEndorsement {
		t.Fatalf("endorsement component %d exceeds max %d", many.EndorsementComponent, maxEndorsement)
	}
}

func TestCompute_TotalCappedAt100(t *testing.T) {
	now := time.Now()
	b := Compute(identity.NodeID{}, Metrics{
		FirstSeen:          now.Add(-365 * 24 * time.Hour),
		Now:                now,
		DirectHubSightings: 10,
		RelaySightings:     10,
		UniqueEndorsers:    50,
	})
	if b.Total > 100 {
		t.Fatalf("total %d exceeds 100", b.Total)
	}
}

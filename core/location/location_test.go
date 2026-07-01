// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.10 - Phase 8 location tests.

package location

import (
	"math"
	"testing"
	"time"
)

func TestHaversineM_SanFranciscoToOakland(t *testing.T) {
	sf := Point{Lat: 37.7749, Lon: -122.4194}
	oak := Point{Lat: 37.8044, Lon: -122.2711}
	d := HaversineM(sf, oak)
	if d < 12_000 || d > 14_000 {
		t.Fatalf("distance = %.0f m, want ~13 km", d)
	}
}

func TestBearingDegrees_North(t *testing.T) {
	a := Point{Lat: 0, Lon: 0}
	b := Point{Lat: 1, Lon: 0}
	bearing := BearingDegrees(a, b)
	if math.Abs(bearing-0) > 1 && math.Abs(bearing-360) > 1 {
		t.Fatalf("bearing = %.1f, want ~0°", bearing)
	}
}

func TestRssiDistanceM_StrongerCloser(t *testing.T) {
	near := RssiDistanceM(-55)
	far := RssiDistanceM(-85)
	if near >= far {
		t.Fatalf("near=%.1f far=%.1f", near, far)
	}
}

func TestComputeOrphanHint_GPS(t *testing.T) {
	ref := Point{Lat: 37.77, Lon: -122.42}
	lat, lon := 37.80, -122.27
	hint := ComputeOrphanHint(ref, time.Now().Add(-time.Minute), time.Now(), &lat, &lon, nil)
	if hint.Source != "gps" || hint.LastPoint == nil || hint.DistanceM <= 0 {
		t.Fatalf("hint = %+v", hint)
	}
}

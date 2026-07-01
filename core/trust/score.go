// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.9 - Phase 7: 0–100 trust score from longevity, proximity, and
//           diminishing-returns endorsements.

package trust

import (
	"math"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
)

const (
	maxLongevity   Score = 35
	maxProximity   Score = 40
	maxEndorsement Score = 25
)

// Metrics are the raw inputs for score calculation.
type Metrics struct {
	FirstSeen          time.Time
	Now                time.Time
	DirectHubSightings int
	RelaySightings     int
	UniqueEndorsers    int
}

// Compute derives a trust Breakdown from network-observed metrics.
func Compute(nodeID identity.NodeID, m Metrics) Breakdown {
	if m.Now.IsZero() {
		m.Now = time.Now()
	}
	longevity := longevityComponent(m.FirstSeen, m.Now)
	proximity := proximityComponent(m.DirectHubSightings, m.RelaySightings)
	endorsement := endorsementComponent(m.UniqueEndorsers)
	total := longevity + proximity + endorsement
	if total > 100 {
		total = 100
	}
	return Breakdown{
		NodeID:               nodeID,
		LongevityComponent:   longevity,
		ProximityComponent:   proximity,
		EndorsementComponent: endorsement,
		Total:                total,
	}
}

func longevityComponent(firstSeen, now time.Time) Score {
	days := now.Sub(firstSeen).Hours() / 24
	if days < 0 {
		days = 0
	}
	// Saturating curve: ~90% of max after 90 days on the network.
	frac := 1 - math.Exp(-days/45)
	return Score(math.Round(float64(maxLongevity) * frac))
}

func proximityComponent(directHub, relay int) Score {
	if directHub < 0 {
		directHub = 0
	}
	if relay < 0 {
		relay = 0
	}
	raw := float64(directHub)*12 + float64(relay)*4
	if raw > float64(maxProximity) {
		raw = float64(maxProximity)
	}
	return Score(math.Round(raw))
}

func endorsementComponent(uniqueEndorsers int) Score {
	if uniqueEndorsers <= 0 {
		return 0
	}
	// Diminishing returns per unique endorser (Sybil dampening).
	frac := math.Log1p(float64(uniqueEndorsers)) / math.Log1p(20)
	s := Score(math.Round(float64(maxEndorsement) * frac))
	if s > maxEndorsement {
		s = maxEndorsement
	}
	return s
}

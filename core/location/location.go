// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.10 - Phase 8: geo math, RSSI ranging, orphan direction hints.

// Package location implements GPS helpers, RSSI distance estimates, and
// bearing/distance hints for orphaned (stale) nodes. See "Location and
// Proximity Estimation" in /plan.md.
package location

import (
	"fmt"
	"math"
	"time"
)

const earthRadiusM = 6_371_000

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64
	Lon float64
}

// Confidence labels how trustworthy a position or hint is.
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// HaversineM returns the great-circle distance in meters between two points.
func HaversineM(a, b Point) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180

	sinDLat := math.Sin(dLat / 2)
	sinDLon := math.Sin(dLon / 2)
	h := sinDLat*sinDLat + math.Cos(lat1)*math.Cos(lat2)*sinDLon*sinDLon
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// BearingDegrees returns the initial compass bearing from a to b (0–360°).
func BearingDegrees(a, b Point) float64 {
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	dLon := (b.Lon - a.Lon) * math.Pi / 180

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	deg := math.Atan2(y, x) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return deg
}

// RssiDistanceM estimates range in meters from an RSSI reading (dBm).
// Uses a log-distance path-loss model calibrated for ~2.4 GHz at 1 m.
func RssiDistanceM(rssi int) float64 {
	const txPowerAt1m = -59.0
	const pathLossN = 2.5
	if rssi >= 0 {
		return 1
	}
	return math.Pow(10, (txPowerAt1m-float64(rssi))/(10*pathLossN))
}

// ReferenceCentroid averages every valid point. Returns false when empty.
func ReferenceCentroid(points []Point) (Point, bool) {
	if len(points) == 0 {
		return Point{}, false
	}
	var sumLat, sumLon float64
	for _, p := range points {
		sumLat += p.Lat
		sumLon += p.Lon
	}
	n := float64(len(points))
	return Point{Lat: sumLat / n, Lon: sumLon / n}, true
}

// AgeConfidence maps how long ago a node was last seen to a confidence band.
func AgeConfidence(lastSeen, now time.Time) (Confidence, string) {
	age := now.Sub(lastSeen)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= 5*time.Minute:
		return ConfidenceHigh, "seen within 5 min"
	case age <= time.Hour:
		return ConfidenceMedium, "seen within 1 hour"
	case age <= 24*time.Hour:
		return ConfidenceMedium, "seen within 24 hours"
	default:
		days := int(age.Hours() / 24)
		if days < 1 {
			days = 1
		}
		return ConfidenceLow, fmt.Sprintf("last seen %d day(s) ago", days)
	}
}

// ProximityEstimate is an RSSI-based guess near a known observer.
type ProximityEstimate struct {
	Observer Point
	Distance float64
	RSSI     int
}

// OrphanHint describes how to reach a stale node from a reference point.
type OrphanHint struct {
	LastPoint      *Point
	BearingDeg     float64
	DistanceM      float64
	Confidence     Confidence
	AgeLabel       string
	Source         string // "gps" or "proximity"
	ProximityNote  string
}

// ComputeOrphanHint builds a direction/distance hint for a stale node.
func ComputeOrphanHint(ref Point, lastSeen time.Time, now time.Time, lastLat, lastLon *float64, prox *ProximityEstimate) OrphanHint {
	conf, ageLabel := AgeConfidence(lastSeen, now)

	if lastLat != nil && lastLon != nil && validCoord(*lastLat, *lastLon) {
		target := Point{Lat: *lastLat, Lon: *lastLon}
		return OrphanHint{
			LastPoint:  &target,
			BearingDeg: BearingDegrees(ref, target),
			DistanceM:  HaversineM(ref, target),
			Confidence: conf,
			AgeLabel:   ageLabel,
			Source:     "gps",
		}
	}

	if prox != nil && validCoord(prox.Observer.Lat, prox.Observer.Lon) {
		dist := prox.Distance
		if dist <= 0 {
			dist = RssiDistanceM(prox.RSSI)
		}
		note := fmt.Sprintf("~%.0f m from observer (RSSI %d dBm)", dist, prox.RSSI)
		return OrphanHint{
			BearingDeg:    BearingDegrees(ref, prox.Observer),
			DistanceM:     HaversineM(ref, prox.Observer) + dist,
			Confidence:    ConfidenceLow,
			AgeLabel:      ageLabel,
			Source:        "proximity",
			ProximityNote: note,
		}
	}

	return OrphanHint{
		Confidence: ConfidenceLow,
		AgeLabel:   ageLabel,
		Source:     "unknown",
	}
}

func validCoord(lat, lon float64) bool {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	if lat == 0 && lon == 0 {
		return false
	}
	return true
}

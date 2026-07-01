// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9: historical_metrics read/write for Monitor charts.

// Package metrics stores time-series samples in historical_metrics.
package metrics

import (
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

const (
	MetricRouteLatencyMs = "route_latency_ms"
)

// Sample is one historical_metrics row.
type Sample struct {
	NodeID     *identity.NodeID
	Value      float64
	RecordedAt time.Time
}

// Store writes and reads historical_metrics.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// Record appends a metric sample. nodeID may be nil for aggregate metrics.
func (s *Store) Record(name string, nodeID *identity.NodeID, value float64, at time.Time) error {
	var nodeBytes any
	if nodeID != nil {
		nodeBytes = nodeID[:]
	}
	_, err := s.db.Exec(
		`INSERT INTO historical_metrics (metric_name, node_id, value, recorded_at) VALUES (?, ?, ?, ?)`,
		name, nodeBytes, value, at.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("metrics: record: %w", err)
	}
	return nil
}

// Query returns samples for metric between since and until inclusive.
func (s *Store) Query(name string, since, until time.Time, limit int) ([]Sample, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.Query(
		`SELECT node_id, value, recorded_at FROM historical_metrics
		 WHERE metric_name = ? AND recorded_at >= ? AND recorded_at <= ?
		 ORDER BY recorded_at DESC LIMIT ?`,
		name, since.UnixMilli(), until.UnixMilli(), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []Sample
	for rows.Next() {
		var nodeBytes []byte
		var value float64
		var recordedMs int64
		if err := rows.Scan(&nodeBytes, &value, &recordedMs); err != nil {
			return nil, err
		}
		samp := Sample{Value: value, RecordedAt: time.UnixMilli(recordedMs)}
		if len(nodeBytes) == len(identity.NodeID{}) {
			var id identity.NodeID
			copy(id[:], nodeBytes)
			samp.NodeID = &id
		}
		samples = append(samples, samp)
	}
	return samples, rows.Err()
}

// LatestValue returns the most recent sample for metric/node, if any.
func (s *Store) LatestValue(name string, nodeID *identity.NodeID) (float64, time.Time, error) {
	var value float64
	var recordedMs int64
	var err error
	if nodeID == nil {
		err = s.db.QueryRow(
			`SELECT value, recorded_at FROM historical_metrics
			 WHERE metric_name = ? AND node_id IS NULL ORDER BY recorded_at DESC LIMIT 1`,
			name,
		).Scan(&value, &recordedMs)
	} else {
		err = s.db.QueryRow(
			`SELECT value, recorded_at FROM historical_metrics
			 WHERE metric_name = ? AND node_id = ? ORDER BY recorded_at DESC LIMIT 1`,
			name, nodeID[:],
		).Scan(&value, &recordedMs)
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	return value, time.UnixMilli(recordedMs), nil
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.9 - Phase 7: proximity_events and trust_endorsements storage.

package trust

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

const (
	TransportHubDirect = "hub-direct"
	TransportHubRelay  = "hub-relay"
)

// Store reads and writes trust-related SQLite tables.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// RecordProximity appends an observer-recorded proximity event.
func (s *Store) RecordProximity(observer, observed identity.NodeID, rssi int, transport string, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO proximity_events (observer_node_id, observed_node_id, rssi, transport, observed_at)
		 VALUES (?, ?, ?, ?, ?)`,
		observer[:], observed[:], rssi, transport, at.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("trust: record proximity: %w", err)
	}
	return nil
}

// ProximityCounts returns direct-hub and relay sighting counts for observed.
func (s *Store) ProximityCounts(observed identity.NodeID) (directHub, relay int, err error) {
	rows, err := s.db.Query(
		`SELECT transport, COUNT(*) FROM proximity_events WHERE observed_node_id = ? GROUP BY transport`,
		observed[:],
	)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var transport string
		var count int
		if err := rows.Scan(&transport, &count); err != nil {
			return 0, 0, err
		}
		switch transport {
		case TransportHubDirect:
			directHub = count
		case TransportHubRelay:
			relay = count
		}
	}
	return directHub, relay, rows.Err()
}

// EndorsementCount returns distinct endorsers for endorsed.
func (s *Store) EndorsementCount(endorsed identity.NodeID) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(DISTINCT endorser_node_id) FROM trust_endorsements WHERE endorsed_node_id = ?`,
		endorsed[:],
	).Scan(&count)
	return count, err
}

// HasProximity reports whether endorser recorded observing endorsed.
func (s *Store) HasProximity(endorser, endorsed identity.NodeID) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM proximity_events WHERE observer_node_id = ? AND observed_node_id = ?`,
		endorser[:], endorsed[:],
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AddEndorsement stores a signed endorsement if proximity exists.
func (s *Store) AddEndorsement(endorser, endorsed identity.NodeID, signature []byte, at time.Time) error {
	ok, err := s.HasProximity(endorser, endorsed)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("trust: endorser has no proximity event with endorsed node")
	}
	_, err = s.db.Exec(
		`INSERT INTO trust_endorsements (endorser_node_id, endorsed_node_id, endorsed_at, signature)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (endorser_node_id, endorsed_node_id) DO UPDATE SET
			 endorsed_at = excluded.endorsed_at,
			 signature = excluded.signature`,
		endorser[:], endorsed[:], at.UnixMilli(), signature,
	)
	if err != nil {
		return fmt.Errorf("trust: add endorsement: %w", err)
	}
	return nil
}

// ScoreForNode computes the trust breakdown for a node using registry first_seen.
func (s *Store) ScoreForNode(nodeID identity.NodeID, firstSeen time.Time, now time.Time) (Breakdown, error) {
	direct, relay, err := s.ProximityCounts(nodeID)
	if err != nil {
		return Breakdown{}, err
	}
	endorsers, err := s.EndorsementCount(nodeID)
	if err != nil {
		return Breakdown{}, err
	}
	return Compute(nodeID, Metrics{
		FirstSeen:          firstSeen,
		Now:                now,
		DirectHubSightings: direct,
		RelaySightings:     relay,
		UniqueEndorsers:    endorsers,
	}), nil
}

// TotalProximityEvents returns how many proximity rows exist for observed.
func (s *Store) TotalProximityEvents(observed identity.NodeID) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM proximity_events WHERE observed_node_id = ?`,
		observed[:],
	).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return count, err
}

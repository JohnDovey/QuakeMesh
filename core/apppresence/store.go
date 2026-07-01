// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10: app_presence read/write and DiscoverPeers queries.

// Package apppresence stores third-party app registrations per node.
package apppresence

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// Record is one app_presence row.
type Record struct {
	NodeID       identity.NodeID
	AppID        string
	AppName      string
	AppVersion   string
	LastReported time.Time
}

// AppStat aggregates active nodes per app_id@version.
type AppStat struct {
	AppID      string
	AppVersion string
	NodeCount  int
	FirstSeen  time.Time
	LastSeen   time.Time
}

// Store reads and writes app_presence.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// Upsert records or refreshes an app's presence on a node.
func (s *Store) Upsert(nodeID identity.NodeID, appID, appName, appVersion string, at time.Time) error {
	if appID == "" {
		return fmt.Errorf("apppresence: app_id required")
	}
	_, err := s.db.Exec(
		`INSERT INTO app_presence (node_id, app_id, app_name, app_version, last_reported)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (node_id, app_id) DO UPDATE SET
		   app_name = excluded.app_name,
		   app_version = excluded.app_version,
		   last_reported = excluded.last_reported`,
		nodeID[:], appID, appName, appVersion, at.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("apppresence: upsert: %w", err)
	}
	return nil
}

// List returns every app_presence row.
func (s *Store) List() ([]Record, error) {
	rows, err := s.db.Query(
		`SELECT node_id, app_id, app_name, app_version, last_reported FROM app_presence ORDER BY last_reported DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []Record
	for rows.Next() {
		var nodeBytes []byte
		var appID, appName, appVersion string
		var reportedMs int64
		if err := rows.Scan(&nodeBytes, &appID, &appName, &appVersion, &reportedMs); err != nil {
			return nil, err
		}
		if len(nodeBytes) != len(identity.NodeID{}) {
			continue
		}
		var id identity.NodeID
		copy(id[:], nodeBytes)
		recs = append(recs, Record{
			NodeID:       id,
			AppID:        appID,
			AppName:      appName,
			AppVersion:   appVersion,
			LastReported: time.UnixMilli(reportedMs),
		})
	}
	return recs, rows.Err()
}

// AppStats returns aggregated counts per app_id and version.
func (s *Store) AppStats() ([]AppStat, error) {
	rows, err := s.db.Query(`
		SELECT app_id, app_version, COUNT(DISTINCT node_id),
		       MIN(last_reported), MAX(last_reported)
		FROM app_presence
		GROUP BY app_id, app_version
		ORDER BY MAX(last_reported) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []AppStat
	for rows.Next() {
		var appID, appVersion string
		var count int
		var firstMs, lastMs int64
		if err := rows.Scan(&appID, &appVersion, &count, &firstMs, &lastMs); err != nil {
			return nil, err
		}
		stats = append(stats, AppStat{
			AppID:      appID,
			AppVersion: appVersion,
			NodeCount:  count,
			FirstSeen:  time.UnixMilli(firstMs),
			LastSeen:   time.UnixMilli(lastMs),
		})
	}
	return stats, rows.Err()
}

// DiscoverPeers returns node IDs running appID matching versionConstraint.
func (s *Store) DiscoverPeers(appID, versionConstraint string) ([]identity.NodeID, error) {
	recs, err := s.List()
	if err != nil {
		return nil, err
	}
	var peers []identity.NodeID
	seen := make(map[identity.NodeID]struct{})
	for _, r := range recs {
		if r.AppID != appID || !VersionMatches(r.AppVersion, versionConstraint) {
			continue
		}
		if _, ok := seen[r.NodeID]; ok {
			continue
		}
		seen[r.NodeID] = struct{}{}
		peers = append(peers, r.NodeID)
	}
	return peers, nil
}

// Get returns one app_presence row for node/app, if present.
func (s *Store) Get(nodeID identity.NodeID, appID string) (Record, bool, error) {
	var appName, appVersion string
	var reportedMs int64
	err := s.db.QueryRow(
		`SELECT app_name, app_version, last_reported FROM app_presence WHERE node_id = ? AND app_id = ?`,
		nodeID[:], appID,
	).Scan(&appName, &appVersion, &reportedMs)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return Record{
		NodeID: nodeID, AppID: appID, AppName: appName,
		AppVersion: appVersion, LastReported: time.UnixMilli(reportedMs),
	}, true, nil
}

// MergeGossip applies a last-writer-wins app_presence update from gossip.
func (s *Store) MergeGossip(nodeID identity.NodeID, appID, appName, appVersion string, reportedAt time.Time) (bool, error) {
	reportedMs := reportedAt.UnixMilli()
	var localMs int64
	err := s.db.QueryRow(
		`SELECT last_reported FROM app_presence WHERE node_id = ? AND app_id = ?`,
		nodeID[:], appID,
	).Scan(&localMs)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return true, s.Upsert(nodeID, appID, appName, appVersion, reportedAt)
	case err != nil:
		return false, err
	default:
		if reportedMs <= localMs {
			return false, nil
		}
		return true, s.Upsert(nodeID, appID, appName, appVersion, reportedAt)
	}
}

// VersionMatches returns true when version satisfies constraint.
// Empty constraint or "*" matches any version; otherwise exact match.
func VersionMatches(version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}
	return version == constraint
}

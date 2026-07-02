// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - LAN segment registry and membership for infrastructure view.

// Package lansegments stores Wi-Fi LAN segments and node/hub membership.
package lansegments

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lancontext"
	"github.com/JohnDovey/QuakeMesh/core/location"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

const (
	EntityNode = "node"
	EntityHub  = "hub"
)

// Segment is one Wi-Fi LAN segment row with member ids.
type Segment struct {
	SegmentID    string
	GatewayIP    string
	SSID         string
	BSSID        string
	FirstSeen    time.Time
	LastSeen     time.Time
	EstimatedLat *float64
	EstimatedLon *float64
	ManualLat    *float64
	ManualLon    *float64
	MapLat       *float64
	MapLon       *float64
	NodeIDs      []string
	HubIDs       []string
}

// Store reads and writes lan_segments and lan_segment_members.
type Store struct {
	db *storage.DB
}

// NewStore wraps a migrated storage.DB.
func NewStore(db *storage.DB) *Store {
	return &Store{db: db}
}

// RecordMembership upserts a segment and links an entity to it.
func (s *Store) RecordMembership(entityType string, entityID identity.NodeID, ctx lancontext.Context, seenAt time.Time) error {
	if !ctx.Valid() {
		return nil
	}
	if entityType != EntityNode && entityType != EntityHub {
		return fmt.Errorf("lansegments: invalid entity type %q", entityType)
	}
	segmentID := lancontext.SegmentID(ctx)
	ms := seenAt.UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("lansegments: begin: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO lan_segments (segment_id, gateway_ip, ssid, bssid, first_seen, last_seen)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (segment_id) DO UPDATE SET
		   gateway_ip = excluded.gateway_ip,
		   ssid = excluded.ssid,
		   bssid = excluded.bssid,
		   last_seen = excluded.last_seen`,
		segmentID, ctx.GatewayIP, nullIfEmpty(ctx.SSID), nullIfEmpty(ctx.BSSID), ms, ms,
	)
	if err != nil {
		return fmt.Errorf("lansegments: upsert segment: %w", err)
	}
	_, err = tx.Exec(
		`INSERT INTO lan_segment_members (segment_id, entity_type, entity_id, local_ip, last_seen)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (segment_id, entity_type, entity_id) DO UPDATE SET
		   local_ip = excluded.local_ip,
		   last_seen = excluded.last_seen`,
		segmentID, entityType, entityID[:], nullIfEmpty(ctx.LocalIP), ms,
	)
	if err != nil {
		return fmt.Errorf("lansegments: upsert member: %w", err)
	}
	if err := s.updateEstimatedPositionTx(tx, segmentID); err != nil {
		return err
	}
	return tx.Commit()
}

// List returns every segment with member node/hub ids and estimated position.
func (s *Store) List() ([]Segment, error) {
	rows, err := s.db.Query(`
		SELECT segment_id, gateway_ip, ssid, bssid, first_seen, last_seen,
		       estimated_lat, estimated_lon, manual_lat, manual_lon
		FROM lan_segments ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var segments []Segment
	for rows.Next() {
		var seg Segment
		var ssid, bssid sql.NullString
		var firstMs, lastMs int64
		var lat, lon, manualLat, manualLon sql.NullFloat64
		if err := rows.Scan(&seg.SegmentID, &seg.GatewayIP, &ssid, &bssid, &firstMs, &lastMs, &lat, &lon, &manualLat, &manualLon); err != nil {
			return nil, err
		}
		if ssid.Valid {
			seg.SSID = ssid.String
		}
		if bssid.Valid {
			seg.BSSID = bssid.String
		}
		seg.FirstSeen = time.UnixMilli(firstMs)
		seg.LastSeen = time.UnixMilli(lastMs)
		if lat.Valid {
			v := lat.Float64
			seg.EstimatedLat = &v
		}
		if lon.Valid {
			v := lon.Float64
			seg.EstimatedLon = &v
		}
		if manualLat.Valid {
			v := manualLat.Float64
			seg.ManualLat = &v
		}
		if manualLon.Valid {
			v := manualLon.Float64
			seg.ManualLon = &v
		}
		seg.MapLat, seg.MapLon = segmentMapPosition(seg.ManualLat, seg.ManualLon, seg.EstimatedLat, seg.EstimatedLon)
		segments = append(segments, seg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range segments {
		nodes, hubs, err := s.membersForSegment(segments[i].SegmentID)
		if err != nil {
			return nil, err
		}
		segments[i].NodeIDs = nodes
		segments[i].HubIDs = hubs
	}
	return segments, nil
}

// SetManualPosition stores operator-placed map coordinates for a segment.
func (s *Store) SetManualPosition(segmentID string, lat, lon float64) error {
	res, err := s.db.Exec(
		`UPDATE lan_segments SET manual_lat = ?, manual_lon = ? WHERE segment_id = ?`,
		lat, lon, segmentID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func segmentMapPosition(manualLat, manualLon, estLat, estLon *float64) (*float64, *float64) {
	if manualLat != nil && manualLon != nil {
		return manualLat, manualLon
	}
	return estLat, estLon
}

func (s *Store) membersForSegment(segmentID string) ([]string, []string, error) {
	rows, err := s.db.Query(
		`SELECT entity_type, entity_id FROM lan_segment_members WHERE segment_id = ? ORDER BY last_seen DESC`,
		segmentID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var nodes, hubs []string
	for rows.Next() {
		var entityType string
		var idBytes []byte
		if err := rows.Scan(&entityType, &idBytes); err != nil {
			return nil, nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		switch entityType {
		case EntityNode:
			nodes = append(nodes, id.String())
		case EntityHub:
			hubs = append(hubs, id.String())
		}
	}
	return nodes, hubs, rows.Err()
}

func (s *Store) updateEstimatedPositionTx(tx *sql.Tx, segmentID string) error {
	rows, err := tx.Query(`
		SELECT nr.last_lat, nr.last_lon
		FROM lan_segment_members m
		JOIN node_registry nr ON nr.node_id = m.entity_id
		WHERE m.segment_id = ?
		  AND m.entity_type = ?
		  AND nr.last_lat IS NOT NULL AND nr.last_lon IS NOT NULL`,
		segmentID, EntityNode,
	)
	if err != nil {
		return fmt.Errorf("lansegments: query member positions: %w", err)
	}
	defer rows.Close()

	var points []location.Point
	for rows.Next() {
		var lat, lon float64
		if err := rows.Scan(&lat, &lon); err != nil {
			return err
		}
		points = append(points, location.Point{Lat: lat, Lon: lon})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(points) == 0 {
		_, err = tx.Exec(`UPDATE lan_segments SET estimated_lat = NULL, estimated_lon = NULL WHERE segment_id = ?`, segmentID)
		return err
	}
	ref, ok := location.ReferenceCentroid(points)
	if !ok {
		return nil
	}
	_, err = tx.Exec(
		`UPDATE lan_segments SET estimated_lat = ?, estimated_lon = ? WHERE segment_id = ?`,
		ref.Lat, ref.Lon, segmentID,
	)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: read-only registry queries plus relay_hubs CRUD
//           for the Monitor dashboard.

// Package datastore provides Monitor-facing access to the Hub's SQLite
// registry (node_registry, routing_table, relay_hubs, dtq_queue).
package datastore

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// Store reads and writes Monitor-relevant tables in quakemeshhub.db.
type Store struct {
	db *storage.DB
}

// New wraps an already-migrated storage.DB shared with QuakeMeshHub.
func New(db *storage.DB) *Store {
	return &Store{db: db}
}

// Node is a dashboard view of node_registry.
type Node struct {
	NodeID    string    `json:"node_id"`
	Status    string    `json:"status"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Lat       *float64  `json:"lat,omitempty"`
	Lon       *float64  `json:"lon,omitempty"`
}

// Route is a dashboard view of routing_table.
type Route struct {
	Destination string  `json:"destination"`
	NextHop     string  `json:"next_hop"`
	TQ          float64 `json:"tq"`
	LatencyMs   int     `json:"latency_ms"`
	HopCount    int     `json:"hop_count"`
}

// RelayHub is a row from relay_hubs.
type RelayHub struct {
	HubID        string    `json:"hub_id"`
	IP           string    `json:"ip"`
	Port         int       `json:"port"`
	Source       string    `json:"source"`
	LastVerified time.Time `json:"last_verified"`
}

// Overview holds aggregate counts for the dashboard.
type Overview struct {
	TotalNodes   int `json:"total_nodes"`
	OnlineNodes  int `json:"online_nodes"`
	OfflineNodes int `json:"offline_nodes"`
	RouteCount   int `json:"route_count"`
	DTNDepth     int `json:"dtn_depth"`
}

// Nodes returns every node in the registry.
func (s *Store) Nodes() ([]Node, error) {
	rows, err := s.db.Query(`SELECT node_id, first_seen, last_seen, last_lat, last_lon, status FROM node_registry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var idBytes []byte
		var firstMs, lastMs int64
		var lat, lon sql.NullFloat64
		var status string
		if err := rows.Scan(&idBytes, &firstMs, &lastMs, &lat, &lon, &status); err != nil {
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		n := Node{
			NodeID:    id.String(),
			Status:    status,
			FirstSeen: time.UnixMilli(firstMs),
			LastSeen:  time.UnixMilli(lastMs),
		}
		if lat.Valid {
			v := lat.Float64
			n.Lat = &v
		}
		if lon.Valid {
			v := lon.Float64
			n.Lon = &v
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// Routes returns every route in the routing table.
func (s *Store) Routes() ([]Route, error) {
	rows, err := s.db.Query(`SELECT destination_node_id, next_hop_node_id, tq, latency_ms, hop_count FROM routing_table`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []Route
	for rows.Next() {
		var dstBytes, hopBytes []byte
		var tq float64
		var latencyMs, hopCount int
		if err := rows.Scan(&dstBytes, &hopBytes, &tq, &latencyMs, &hopCount); err != nil {
			return nil, err
		}
		var dst, hop identity.NodeID
		copy(dst[:], dstBytes)
		copy(hop[:], hopBytes)
		routes = append(routes, Route{
			Destination: dst.String(),
			NextHop:     hop.String(),
			TQ:          tq,
			LatencyMs:   latencyMs,
			HopCount:    hopCount,
		})
	}
	return routes, rows.Err()
}

// OverviewSnapshot returns current aggregate counts.
func (s *Store) OverviewSnapshot() (Overview, error) {
	var o Overview
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM node_registry`).Scan(&o.TotalNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE status = 'online'`).Scan(&o.OnlineNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM node_registry WHERE status != 'online'`).Scan(&o.OfflineNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM routing_table`).Scan(&o.RouteCount); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM dtq_queue WHERE expires_at > ?`,
		time.Now().UnixMilli(),
	).Scan(&o.DTNDepth); err != nil {
		return o, err
	}
	return o, nil
}

// RelayHubs returns every relay hub record.
func (s *Store) RelayHubs() ([]RelayHub, error) {
	rows, err := s.db.Query(`SELECT hub_id, ip, port, source, last_verified FROM relay_hubs ORDER BY last_verified DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hubs []RelayHub
	for rows.Next() {
		var idBytes []byte
		var ip, source string
		var port int
		var verifiedMs int64
		if err := rows.Scan(&idBytes, &ip, &port, &source, &verifiedMs); err != nil {
			return nil, err
		}
		var hubID identity.NodeID
		copy(hubID[:], idBytes)
		hubs = append(hubs, RelayHub{
			HubID:        hubID.String(),
			IP:           ip,
			Port:         port,
			Source:       source,
			LastVerified: time.UnixMilli(verifiedMs),
		})
	}
	return hubs, rows.Err()
}

// AddRelayHub inserts or replaces a manually added relay hub entry.
func (s *Store) AddRelayHub(ip string, port int) (RelayHub, error) {
	hubID := relayHubID(ip, port)
	now := time.Now()
	_, err := s.db.Exec(
		`INSERT INTO relay_hubs (hub_id, ip, port, source, last_verified) VALUES (?, ?, ?, 'manual', ?)
		 ON CONFLICT (hub_id) DO UPDATE SET ip = excluded.ip, port = excluded.port, source = 'manual', last_verified = excluded.last_verified`,
		hubID[:], ip, port, now.UnixMilli(),
	)
	if err != nil {
		return RelayHub{}, fmt.Errorf("datastore: add relay hub: %w", err)
	}
	return RelayHub{
		HubID:        hubID.String(),
		IP:           ip,
		Port:         port,
		Source:       "manual",
		LastVerified: now,
	}, nil
}

// RemoveRelayHub deletes a relay hub by id.
func (s *Store) RemoveRelayHub(hubID identity.NodeID) error {
	_, err := s.db.Exec(`DELETE FROM relay_hubs WHERE hub_id = ?`, hubID[:])
	return err
}

// MarkRelayHubVerified updates last_verified for a relay hub.
func (s *Store) MarkRelayHubVerified(hubID identity.NodeID, verifiedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE relay_hubs SET last_verified = ? WHERE hub_id = ?`, verifiedAt.UnixMilli(), hubID[:])
	return err
}

// RelayHubByID returns one relay hub row.
func (s *Store) RelayHubByID(hubID identity.NodeID) (RelayHub, error) {
	var ip, source string
	var port int
	var verifiedMs int64
	err := s.db.QueryRow(
		`SELECT ip, port, source, last_verified FROM relay_hubs WHERE hub_id = ?`,
		hubID[:],
	).Scan(&ip, &port, &source, &verifiedMs)
	if err != nil {
		return RelayHub{}, err
	}
	return RelayHub{
		HubID:        hubID.String(),
		IP:           ip,
		Port:         port,
		Source:       source,
		LastVerified: time.UnixMilli(verifiedMs),
	}, nil
}

func relayHubID(ip string, port int) identity.NodeID {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", ip, port)))
	var id identity.NodeID
	copy(id[:], sum[:])
	return id
}

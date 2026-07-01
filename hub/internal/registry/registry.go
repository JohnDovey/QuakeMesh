// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial registry: node_registry/routing_table access for
//           the Phase 2 OGM engine and management API.
//   0.0.6 - GetRoute and DeleteRoutesViaNextHop for Phase 5 failover.
//   0.0.7 - hub_registry access for backbone hub tracking in Monitor.

// Package registry provides typed access to the node_registry and
// routing_table tables (see /core/storage) backing QuakeMeshHub.
package registry

import (
	"database/sql"
	"errors"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

// Registry provides typed access to a Hub's SQLite-backed registry.
type Registry struct {
	db *storage.DB
}

// New wraps an already-migrated storage.DB.
func New(db *storage.DB) *Registry {
	return &Registry{db: db}
}

// NodeStatus mirrors node_registry.status.
type NodeStatus string

const (
	NodeStatusOnline NodeStatus = "online"
	NodeStatusStale  NodeStatus = "stale"
)

// Node is a row from node_registry.
type Node struct {
	NodeID    identity.NodeID
	FirstSeen time.Time
	LastSeen  time.Time
	Status    NodeStatus
	LastLat   *float64
	LastLon   *float64
}

// Route is a row from routing_table.
type Route struct {
	Destination identity.NodeID
	NextHop     identity.NodeID
	TQ          float64
	LatencyMs   int
	HopCount    int
}

// UpsertSeen records that nodeID was heard from at seenAt: creating the
// node_registry row (status online) if this is the first time it's been
// seen, or refreshing last_seen and reviving status to online otherwise.
// It reports whether the node's status transitioned to online (a brand
// new node, or one reviving from stale) so callers can decide whether to
// emit a NodeStatusChanged event -- a plain timestamp refresh of an
// already-online node does not count as a change.
func (r *Registry) UpsertSeen(nodeID identity.NodeID, seenAt time.Time) (statusChanged bool, err error) {
	ts := seenAt.UnixMilli()

	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var status string
	err = tx.QueryRow(`SELECT status FROM node_registry WHERE node_id = ?`, nodeID[:]).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO node_registry (node_id, first_seen, last_seen, status) VALUES (?, ?, ?, 'online')`,
			nodeID[:], ts, ts,
		); err != nil {
			return false, err
		}
		statusChanged = true
	case err != nil:
		return false, err
	default:
		if _, err := tx.Exec(
			`UPDATE node_registry SET last_seen = ?, status = 'online' WHERE node_id = ?`,
			ts, nodeID[:],
		); err != nil {
			return false, err
		}
		statusChanged = status != string(NodeStatusOnline)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return statusChanged, nil
}

// UpdateLocation stores the latest self-reported GPS fix for a node.
func (r *Registry) UpdateLocation(nodeID identity.NodeID, lat, lon float64, observedAt time.Time) error {
	_, err := r.db.Exec(
		`UPDATE node_registry SET last_lat = ?, last_lon = ?, last_seen = MAX(last_seen, ?) WHERE node_id = ?`,
		lat, lon, observedAt.UnixMilli(), nodeID[:],
	)
	return err
}

// MarkStaleBefore marks every online node last seen before cutoff as
// stale, returning the ids of the nodes that changed.
func (r *Registry) MarkStaleBefore(cutoff time.Time) ([]identity.NodeID, error) {
	cutoffMs := cutoff.UnixMilli()

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	rows, err := tx.Query(`SELECT node_id FROM node_registry WHERE status = 'online' AND last_seen < ?`, cutoffMs)
	if err != nil {
		return nil, err
	}
	var stale []identity.NodeID
	for rows.Next() {
		var idBytes []byte
		if err := rows.Scan(&idBytes); err != nil {
			rows.Close()
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		stale = append(stale, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(stale) > 0 {
		if _, err := tx.Exec(
			`UPDATE node_registry SET status = 'stale' WHERE status = 'online' AND last_seen < ?`, cutoffMs,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return stale, nil
}

// Nodes returns every row in node_registry.
func (r *Registry) Nodes() ([]Node, error) {
	rows, err := r.db.Query(`SELECT node_id, first_seen, last_seen, last_lat, last_lon, status FROM node_registry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var idBytes []byte
		var firstSeenMs, lastSeenMs int64
		var lat, lon sql.NullFloat64
		var status string
		if err := rows.Scan(&idBytes, &firstSeenMs, &lastSeenMs, &lat, &lon, &status); err != nil {
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		n := Node{
			NodeID:    id,
			FirstSeen: time.UnixMilli(firstSeenMs),
			LastSeen:  time.UnixMilli(lastSeenMs),
			Status:    NodeStatus(status),
		}
		if lat.Valid {
			v := lat.Float64
			n.LastLat = &v
		}
		if lon.Valid {
			v := lon.Float64
			n.LastLon = &v
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// UpsertRoute inserts or updates the best known route to route.Destination.
func (r *Registry) UpsertRoute(route Route) error {
	_, err := r.db.Exec(
		`INSERT INTO routing_table (destination_node_id, next_hop_node_id, tq, latency_ms, hop_count)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (destination_node_id) DO UPDATE SET
			 next_hop_node_id = excluded.next_hop_node_id,
			 tq = excluded.tq,
			 latency_ms = excluded.latency_ms,
			 hop_count = excluded.hop_count`,
		route.Destination[:], route.NextHop[:], route.TQ, route.LatencyMs, route.HopCount,
	)
	return err
}

// Routes returns every row in routing_table.
func (r *Registry) Routes() ([]Route, error) {
	rows, err := r.db.Query(`SELECT destination_node_id, next_hop_node_id, tq, latency_ms, hop_count FROM routing_table`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []Route
	for rows.Next() {
		var dstBytes, nextHopBytes []byte
		var route Route
		if err := rows.Scan(&dstBytes, &nextHopBytes, &route.TQ, &route.LatencyMs, &route.HopCount); err != nil {
			return nil, err
		}
		copy(route.Destination[:], dstBytes)
		copy(route.NextHop[:], nextHopBytes)
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

// GetRoute returns the stored route to destination, if any.
func (r *Registry) GetRoute(destination identity.NodeID) (Route, bool, error) {
	var dstBytes, nextHopBytes []byte
	var route Route
	err := r.db.QueryRow(
		`SELECT destination_node_id, next_hop_node_id, tq, latency_ms, hop_count FROM routing_table WHERE destination_node_id = ?`,
		destination[:],
	).Scan(&dstBytes, &nextHopBytes, &route.TQ, &route.LatencyMs, &route.HopCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Route{}, false, nil
		}
		return Route{}, false, err
	}
	copy(route.Destination[:], dstBytes)
	copy(route.NextHop[:], nextHopBytes)
	return route, true, nil
}

// DeleteRoutesViaNextHop removes every route that used nextHop as its
// first hop (failover when a neighbor goes stale).
func (r *Registry) DeleteRoutesViaNextHop(nextHop identity.NodeID) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM routing_table WHERE next_hop_node_id = ?`, nextHop[:])
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// HubStatus mirrors hub_registry.status.
type HubStatus string

const (
	HubStatusOnline HubStatus = "online"
	HubStatusStale  HubStatus = "stale"
)

// Hub is a row from hub_registry.
type Hub struct {
	HubID        identity.NodeID
	LastIP       string
	LastPort     int
	RelayCapable bool
	FirstSeen    time.Time
	LastSeen     time.Time
	Status       HubStatus
}

// UpsertHubSeen records a direct backbone hub contact at seenAt, creating
// or refreshing the hub_registry row. It reports whether status changed to
// online (new hub or revival from stale).
func (r *Registry) UpsertHubSeen(hubID identity.NodeID, ip string, port int, seenAt time.Time) (statusChanged bool, err error) {
	ts := seenAt.UnixMilli()

	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var status string
	err = tx.QueryRow(`SELECT status FROM hub_registry WHERE hub_id = ?`, hubID[:]).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(
			`INSERT INTO hub_registry (hub_id, last_ip, last_port, relay_capable, first_seen, last_seen, status)
			 VALUES (?, ?, ?, 0, ?, ?, 'online')`,
			hubID[:], ip, port, ts, ts,
		); err != nil {
			return false, err
		}
		statusChanged = true
	case err != nil:
		return false, err
	default:
		if _, err := tx.Exec(
			`UPDATE hub_registry SET last_ip = ?, last_port = ?, last_seen = ?, status = 'online' WHERE hub_id = ?`,
			ip, port, ts, hubID[:],
		); err != nil {
			return false, err
		}
		statusChanged = status != string(HubStatusOnline)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return statusChanged, nil
}

// MarkHubsStaleBefore marks every online hub last seen before cutoff as
// stale, returning the ids that changed.
func (r *Registry) MarkHubsStaleBefore(cutoff time.Time) ([]identity.NodeID, error) {
	cutoffMs := cutoff.UnixMilli()

	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	rows, err := tx.Query(`SELECT hub_id FROM hub_registry WHERE status = 'online' AND last_seen < ?`, cutoffMs)
	if err != nil {
		return nil, err
	}
	var stale []identity.NodeID
	for rows.Next() {
		var idBytes []byte
		if err := rows.Scan(&idBytes); err != nil {
			rows.Close()
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		stale = append(stale, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(stale) > 0 {
		if _, err := tx.Exec(
			`UPDATE hub_registry SET status = 'stale' WHERE status = 'online' AND last_seen < ?`, cutoffMs,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return stale, nil
}

// Hubs returns every row in hub_registry.
func (r *Registry) Hubs() ([]Hub, error) {
	rows, err := r.db.Query(
		`SELECT hub_id, last_ip, last_port, relay_capable, first_seen, last_seen, status FROM hub_registry ORDER BY last_seen DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hubs []Hub
	for rows.Next() {
		var idBytes []byte
		var ip sql.NullString
		var port sql.NullInt64
		var relay int
		var firstMs, lastMs int64
		var status string
		if err := rows.Scan(&idBytes, &ip, &port, &relay, &firstMs, &lastMs, &status); err != nil {
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		h := Hub{
			HubID:        id,
			RelayCapable: relay != 0,
			FirstSeen:    time.UnixMilli(firstMs),
			LastSeen:     time.UnixMilli(lastMs),
			Status:       HubStatus(status),
		}
		if ip.Valid {
			h.LastIP = ip.String
		}
		if port.Valid {
			h.LastPort = int(port.Int64)
		}
		hubs = append(hubs, h)
	}
	return hubs, rows.Err()
}

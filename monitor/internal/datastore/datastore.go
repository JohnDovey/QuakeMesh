// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: read-only registry queries plus relay_hubs CRUD
//           for the Monitor dashboard.
//   0.0.7 - hub_registry queries; nodes exclude backbone hubs.
//   0.0.9 - Trust score breakdown per mesh node.
//   0.0.10 - Orphan direction hints for stale nodes on the Node Map.
//   0.0.11 - Hop latency history and internet-fallback config for Monitor.
//   0.0.12 - App Stats from app_presence table.
//   0.0.13 - Ban list proposals, verdicts, and local enforcement status.
//   0.0.19 - LAN infrastructure segments for Wi-Fi gateway view.

// Package datastore provides Monitor-facing access to the Hub's SQLite
// registry (node_registry, hub_registry, routing_table, relay_hubs,
// dtq_queue).
package datastore

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/banlist"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
	"github.com/JohnDovey/QuakeMesh/core/location"
	"github.com/JohnDovey/QuakeMesh/core/metrics"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/core/trust"
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

// Hub is a dashboard view of hub_registry.
type Hub struct {
	HubID              string    `json:"hub_id"`
	LastIP             string    `json:"last_ip,omitempty"`
	LastPort           int       `json:"last_port,omitempty"`
	RelayCapable       bool      `json:"relay_capable"`
	Status             string    `json:"status"`
	FirstSeen          time.Time `json:"first_seen"`
	LastSeen           time.Time `json:"last_seen"`
	ManualLat          *float64  `json:"manual_lat,omitempty"`
	ManualLon          *float64  `json:"manual_lon,omitempty"`
	Lat                *float64  `json:"lat,omitempty"`
	Lon                *float64  `json:"lon,omitempty"`
	LocalHubEndorsed   bool      `json:"local_hub_endorsed"`
	EndorsementCount   int       `json:"endorsement_count"`
}

// Overview holds aggregate counts for the dashboard.
type Overview struct {
	TotalNodes    int `json:"total_nodes"`
	OnlineNodes   int `json:"online_nodes"`
	OfflineNodes  int `json:"offline_nodes"`
	TotalHubs     int `json:"total_hubs"`
	OnlineHubs    int `json:"online_hubs"`
	OfflineHubs   int `json:"offline_hubs"`
	RouteCount    int  `json:"route_count"`
	DTNDepth      int  `json:"dtn_depth"`
	InternetFallback bool `json:"internet_fallback_enabled"`
}

// HopLatencyPoint is one route_latency_ms sample for charts.
type HopLatencyPoint struct {
	RecordedAt time.Time `json:"recorded_at"`
	Value      float64   `json:"value"`
	NodeID     string    `json:"node_id,omitempty"`
}

// TrustScore is a per-node trust breakdown for the dashboard.
type TrustScore struct {
	NodeID            string `json:"node_id"`
	Status            string `json:"status"`
	Longevity         int    `json:"longevity"`
	Proximity         int    `json:"proximity"`
	Endorsements      int    `json:"endorsements"`
	Total             int    `json:"total"`
	ProximityEvents   int    `json:"proximity_events"`
	EndorsementCount  int    `json:"endorsement_count"`
	LocalHubEndorsed  bool   `json:"local_hub_endorsed"`
}

// OrphanHint is a bearing/distance estimate for a stale mesh node.
type OrphanHint struct {
	NodeID        string    `json:"node_id"`
	Status        string    `json:"status"`
	LastSeen      time.Time `json:"last_seen"`
	LastLat       *float64  `json:"last_lat,omitempty"`
	LastLon       *float64  `json:"last_lon,omitempty"`
	RefLat        float64   `json:"ref_lat"`
	RefLon        float64   `json:"ref_lon"`
	BearingDeg    float64   `json:"bearing_deg"`
	DistanceM     float64   `json:"distance_m"`
	Confidence    string    `json:"confidence"`
	AgeLabel      string    `json:"age_label"`
	Source        string    `json:"source"`
	ProximityNote string    `json:"proximity_note,omitempty"`
}

// AppStat is an aggregated app_id@version row for the App Stats view.
type AppStat struct {
	AppID      string    `json:"app_id"`
	AppVersion string    `json:"app_version"`
	NodeCount  int       `json:"node_count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
}

// InfrastructureSegment is a Wi-Fi LAN segment with connected members.
type InfrastructureSegment struct {
	SegmentID    string    `json:"segment_id"`
	GatewayIP    string    `json:"gateway_ip"`
	SSID         string    `json:"ssid,omitempty"`
	BSSID        string    `json:"bssid,omitempty"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	EstimatedLat *float64  `json:"estimated_lat,omitempty"`
	EstimatedLon *float64  `json:"estimated_lon,omitempty"`
	ManualLat    *float64  `json:"manual_lat,omitempty"`
	ManualLon    *float64  `json:"manual_lon,omitempty"`
	MapLat       *float64  `json:"map_lat,omitempty"`
	MapLon       *float64  `json:"map_lon,omitempty"`
	NodeIDs      []string  `json:"node_ids"`
	HubIDs       []string  `json:"hub_ids"`
	MemberCount  int       `json:"member_count"`
}

// Nodes returns mesh nodes, excluding backbone hubs in hub_registry.
func (s *Store) Nodes() ([]Node, error) {
	rows, err := s.db.Query(`
		SELECT node_id, first_seen, last_seen, last_lat, last_lon, status
		FROM node_registry
		WHERE node_id NOT IN (SELECT hub_id FROM hub_registry)
		ORDER BY last_seen DESC`)
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

// Hubs returns every backbone hub in hub_registry.
func (s *Store) Hubs() ([]Hub, error) {
	rows, err := s.db.Query(`
		SELECT hub_id, last_ip, last_port, relay_capable, first_seen, last_seen, status,
		       manual_lat, manual_lon
		FROM hub_registry ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hubs []Hub
	trustStore := trust.NewStore(s.db)
	localHub, _ := s.LocalHubID()
	for rows.Next() {
		var idBytes []byte
		var ip sql.NullString
		var port sql.NullInt64
		var relay int
		var firstMs, lastMs int64
		var status string
		var manualLat, manualLon sql.NullFloat64
		if err := rows.Scan(&idBytes, &ip, &port, &relay, &firstMs, &lastMs, &status, &manualLat, &manualLon); err != nil {
			return nil, err
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		h := Hub{
			HubID:        id.String(),
			RelayCapable: relay != 0,
			Status:       status,
			FirstSeen:    time.UnixMilli(firstMs),
			LastSeen:     time.UnixMilli(lastMs),
		}
		if ip.Valid {
			h.LastIP = ip.String
		}
		if port.Valid {
			h.LastPort = int(port.Int64)
		}
		if manualLat.Valid {
			v := manualLat.Float64
			h.ManualLat = &v
			h.Lat = &v
		}
		if manualLon.Valid {
			v := manualLon.Float64
			h.ManualLon = &v
			h.Lon = &v
		}
		if h.Lat == nil {
			if nrLat, nrLon, ok := s.nodeRegistryCoords(id); ok {
				h.Lat, h.Lon = &nrLat, &nrLon
			}
		}
		if count, err := trustStore.EndorsementCount(id); err == nil {
			h.EndorsementCount = count
		}
		if localHub != (identity.NodeID{}) {
			if endorsed, err := trustStore.HasEndorsement(localHub, id); err == nil {
				h.LocalHubEndorsed = endorsed
			}
		}
		hubs = append(hubs, h)
	}
	return hubs, rows.Err()
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
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM node_registry
		WHERE node_id NOT IN (SELECT hub_id FROM hub_registry)`).Scan(&o.TotalNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM node_registry
		WHERE status = 'online' AND node_id NOT IN (SELECT hub_id FROM hub_registry)`).Scan(&o.OnlineNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM node_registry
		WHERE status != 'online' AND node_id NOT IN (SELECT hub_id FROM hub_registry)`).Scan(&o.OfflineNodes); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hub_registry`).Scan(&o.TotalHubs); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hub_registry WHERE status = 'online'`).Scan(&o.OnlineHubs); err != nil {
		return o, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM hub_registry WHERE status != 'online'`).Scan(&o.OfflineHubs); err != nil {
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
	var fbErr error
	o.InternetFallback, fbErr = s.InternetFallbackEnabled()
	if fbErr != nil {
		return o, fbErr
	}
	return o, nil
}

// TrustScores returns trust breakdowns for every mesh node.
func (s *Store) TrustScores() ([]TrustScore, error) {
	nodes, err := s.Nodes()
	if err != nil {
		return nil, err
	}
	trustStore := trust.NewStore(s.db)
	now := time.Now()
	localHub, _ := s.LocalHubID()
	scores := make([]TrustScore, 0, len(nodes))
	for _, n := range nodes {
		idBytes, err := hex.DecodeString(n.NodeID)
		if err != nil || len(idBytes) != len(identity.NodeID{}) {
			continue
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		b, err := trustStore.ScoreForNode(id, n.FirstSeen, now)
		if err != nil {
			return nil, err
		}
		proxEvents, err := trustStore.TotalProximityEvents(id)
		if err != nil {
			return nil, err
		}
		endorsers, err := trustStore.EndorsementCount(id)
		if err != nil {
			return nil, err
		}
		localEndorsed := false
		if localHub != (identity.NodeID{}) {
			localEndorsed, _ = trustStore.HasEndorsement(localHub, id)
		}
		scores = append(scores, TrustScore{
			NodeID:           n.NodeID,
			Status:           n.Status,
			Longevity:        int(b.LongevityComponent),
			Proximity:        int(b.ProximityComponent),
			Endorsements:     int(b.EndorsementComponent),
			Total:            int(b.Total),
			ProximityEvents:  proxEvents,
			EndorsementCount: endorsers,
			LocalHubEndorsed: localEndorsed,
		})
	}
	return scores, nil
}

// OrphanHints returns bearing/distance estimates for stale mesh nodes.
func (s *Store) OrphanHints() ([]OrphanHint, error) {
	nodes, err := s.Nodes()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var refPoints []location.Point
	for _, n := range nodes {
		if n.Status != "online" || n.Lat == nil || n.Lon == nil {
			continue
		}
		refPoints = append(refPoints, location.Point{Lat: *n.Lat, Lon: *n.Lon})
	}
	ref, hasRef := location.ReferenceCentroid(refPoints)
	if !hasRef {
		ref = location.Point{Lat: 0, Lon: 0}
	}

	var hints []OrphanHint
	for _, n := range nodes {
		if n.Status == "online" {
			continue
		}
		idBytes, err := hex.DecodeString(n.NodeID)
		if err != nil || len(idBytes) != len(identity.NodeID{}) {
			continue
		}
		var id identity.NodeID
		copy(id[:], idBytes)
		prox, _ := s.latestProximityEstimate(id)
		computed := location.ComputeOrphanHint(ref, n.LastSeen, now, n.Lat, n.Lon, prox)
		h := OrphanHint{
			NodeID:        n.NodeID,
			Status:        n.Status,
			LastSeen:      n.LastSeen,
			LastLat:       n.Lat,
			LastLon:       n.Lon,
			RefLat:        ref.Lat,
			RefLon:        ref.Lon,
			BearingDeg:    computed.BearingDeg,
			DistanceM:     computed.DistanceM,
			Confidence:    string(computed.Confidence),
			AgeLabel:      computed.AgeLabel,
			Source:        computed.Source,
			ProximityNote: computed.ProximityNote,
		}
		if computed.LastPoint != nil {
			h.LastLat = &computed.LastPoint.Lat
			h.LastLon = &computed.LastPoint.Lon
		}
		hints = append(hints, h)
	}
	return hints, nil
}

func (s *Store) latestProximityEstimate(observed identity.NodeID) (*location.ProximityEstimate, error) {
	var rssi int
	var obsLat, obsLon sql.NullFloat64
	err := s.db.QueryRow(`
		SELECT pe.rssi, nr.last_lat, nr.last_lon
		FROM proximity_events pe
		JOIN node_registry nr ON nr.node_id = pe.observer_node_id
		WHERE pe.observed_node_id = ?
		  AND nr.last_lat IS NOT NULL AND nr.last_lon IS NOT NULL
		ORDER BY pe.observed_at DESC LIMIT 1`,
		observed[:],
	).Scan(&rssi, &obsLat, &obsLon)
	if err != nil {
		return nil, err
	}
	if !obsLat.Valid || !obsLon.Valid {
		return nil, sql.ErrNoRows
	}
	dist := location.RssiDistanceM(rssi)
	return &location.ProximityEstimate{
		Observer: location.Point{Lat: obsLat.Float64, Lon: obsLon.Float64},
		Distance: dist,
		RSSI:     rssi,
	}, nil
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

const configKeyInternetFallback = "internet_fallback_enabled"

// InternetFallbackEnabled reads the hub config toggle.
func (s *Store) InternetFallbackEnabled() (bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, configKeyInternetFallback).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, nil
	}
	return parsed, nil
}

// SetInternetFallbackEnabled updates the hub config toggle.
func (s *Store) SetInternetFallbackEnabled(enabled bool) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		configKeyInternetFallback, strconv.FormatBool(enabled),
	)
	return err
}

// HopLatency returns recent route_latency_ms samples within the window.
func (s *Store) HopLatency(window time.Duration, limit int) ([]HopLatencyPoint, error) {
	until := time.Now()
	since := until.Add(-window)
	samples, err := metrics.NewStore(s.db).Query(metrics.MetricRouteLatencyMs, since, until, limit)
	if err != nil {
		return nil, err
	}
	points := make([]HopLatencyPoint, 0, len(samples))
	for _, samp := range samples {
		pt := HopLatencyPoint{RecordedAt: samp.RecordedAt, Value: samp.Value}
		if samp.NodeID != nil {
			pt.NodeID = samp.NodeID.String()
		}
		points = append(points, pt)
	}
	return points, nil
}

// AppStats returns aggregated third-party app presence across the network.
func (s *Store) AppStats() ([]AppStat, error) {
	stats, err := apppresence.NewStore(s.db).AppStats()
	if err != nil {
		return nil, err
	}
	out := make([]AppStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, AppStat{
			AppID:      st.AppID,
			AppVersion: st.AppVersion,
			NodeCount:  st.NodeCount,
			FirstSeen:  st.FirstSeen,
			LastSeen:   st.LastSeen,
		})
	}
	return out, nil
}

const configKeyLocalHubID = "local_hub_id"

// BanEntry is a ban proposal with network-wide verdict tallies.
type BanEntry struct {
	BanID           string    `json:"ban_id"`
	AppID           string    `json:"app_id"`
	VersionRange    string    `json:"version_range"`
	Reason          string    `json:"reason"`
	ProposedBy      string    `json:"proposed_by"`
	ProposedAt      time.Time `json:"proposed_at"`
	AgreeCount      int       `json:"agree_count"`
	DisagreeCount   int       `json:"disagree_count"`
	LocalVerdict    string    `json:"local_verdict,omitempty"`
	LocallyEnforced bool      `json:"locally_enforced"`
}

// LocalHubID returns this hub's node id from the config table.
func (s *Store) LocalHubID() (identity.NodeID, error) {
	var id identity.NodeID
	var value string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, configKeyLocalHubID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) || value == "" {
		return id, fmt.Errorf("datastore: local hub id not set")
	}
	bytes, err := hex.DecodeString(value)
	if err != nil || len(bytes) != len(id) {
		return id, fmt.Errorf("datastore: invalid local hub id")
	}
	copy(id[:], bytes)
	return id, nil
}

// BanList returns proposals with verdict tallies and local hub status.
func (s *Store) BanList() ([]BanEntry, error) {
	store := banlist.NewStore(s.db)
	proposals, err := store.ListProposals()
	if err != nil {
		return nil, err
	}
	localHub, hasLocal := identity.NodeID{}, false
	if id, err := s.LocalHubID(); err == nil {
		localHub = id
		hasLocal = true
	}
	entries := make([]BanEntry, 0, len(proposals))
	for _, p := range proposals {
		tally, err := store.TallyVerdicts(p.BanID)
		if err != nil {
			return nil, err
		}
		e := BanEntry{
			BanID:         hex.EncodeToString(p.BanID[:]),
			AppID:         p.AppID,
			VersionRange:  p.VersionRange,
			Reason:        p.Reason,
			ProposedBy:    p.ProposedBy.String(),
			ProposedAt:    p.ProposedAt,
			AgreeCount:    tally.Agree,
			DisagreeCount: tally.Disagree,
		}
		if hasLocal {
			if v, ok, err := store.VerdictForHub(p.BanID, localHub); err == nil && ok {
				if v.Agree {
					e.LocalVerdict = "agree"
					e.LocallyEnforced = true
				} else {
					e.LocalVerdict = "disagree"
				}
			}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Infrastructure returns Wi-Fi LAN segments and their node/hub members.
func (s *Store) Infrastructure() ([]InfrastructureSegment, error) {
	rows, err := lansegments.NewStore(s.db).List()
	if err != nil {
		return nil, err
	}
	out := make([]InfrastructureSegment, 0, len(rows))
	for _, row := range rows {
		seg := InfrastructureSegment{
			SegmentID: row.SegmentID,
			GatewayIP: row.GatewayIP,
			SSID:      row.SSID,
			BSSID:     row.BSSID,
			FirstSeen: row.FirstSeen,
			LastSeen:  row.LastSeen,
			NodeIDs:   row.NodeIDs,
			HubIDs:    row.HubIDs,
		}
		if row.EstimatedLat != nil {
			v := *row.EstimatedLat
			seg.EstimatedLat = &v
		}
		if row.EstimatedLon != nil {
			v := *row.EstimatedLon
			seg.EstimatedLon = &v
		}
		if row.ManualLat != nil {
			v := *row.ManualLat
			seg.ManualLat = &v
		}
		if row.ManualLon != nil {
			v := *row.ManualLon
			seg.ManualLon = &v
		}
		if row.MapLat != nil {
			v := *row.MapLat
			seg.MapLat = &v
		}
		if row.MapLon != nil {
			v := *row.MapLon
			seg.MapLon = &v
		}
		seg.MemberCount = len(row.NodeIDs) + len(row.HubIDs)
		out = append(out, seg)
	}
	return out, nil
}

func (s *Store) nodeRegistryCoords(nodeID identity.NodeID) (lat, lon float64, ok bool) {
	var latN, lonN sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT last_lat, last_lon FROM node_registry WHERE node_id = ?`,
		nodeID[:],
	).Scan(&latN, &lonN)
	if err != nil || !latN.Valid || !lonN.Valid {
		return 0, 0, false
	}
	return latN.Float64, lonN.Float64, true
}

// EndorseEntity records an endorsement from this hub for a node or hub id.
func (s *Store) EndorseEntity(endorsedHex string) error {
	localHub, err := s.LocalHubID()
	if err != nil {
		return err
	}
	idBytes, err := hex.DecodeString(endorsedHex)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		return fmt.Errorf("invalid node id")
	}
	var endorsed identity.NodeID
	copy(endorsed[:], idBytes)
	return trust.NewStore(s.db).EndorseWithHubContact(localHub, endorsed, time.Now())
}

// SetHubManualLocation stores operator map coordinates for a backbone hub.
func (s *Store) SetHubManualLocation(hubHex string, lat, lon float64) error {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fmt.Errorf("invalid coordinates")
	}
	idBytes, err := hex.DecodeString(hubHex)
	if err != nil || len(idBytes) != len(identity.NodeID{}) {
		return fmt.Errorf("invalid hub id")
	}
	var hubID identity.NodeID
	copy(hubID[:], idBytes)
	res, err := s.db.Exec(
		`UPDATE hub_registry SET manual_lat = ?, manual_lon = ? WHERE hub_id = ?`,
		lat, lon, hubID[:],
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

// SetInfrastructureManualLocation stores operator map coordinates for a LAN segment.
func (s *Store) SetInfrastructureManualLocation(segmentID string, lat, lon float64) error {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fmt.Errorf("invalid coordinates")
	}
	return lansegments.NewStore(s.db).SetManualPosition(segmentID, lat, lon)
}

// ProposeBan creates a new ban proposal from the local hub.
func (s *Store) ProposeBan(appID, versionRange, reason string) (BanEntry, error) {
	localHub, err := s.LocalHubID()
	if err != nil {
		return BanEntry{}, err
	}
	if versionRange == "" {
		versionRange = "*"
	}
	p := banlist.Proposal{
		AppID: appID, VersionRange: versionRange, Reason: reason,
		ProposedBy: localHub, ProposedAt: time.Now(),
	}
	if err := banlist.NewStore(s.db).Propose(p); err != nil {
		return BanEntry{}, err
	}
	list, err := s.BanList()
	if err != nil {
		return BanEntry{}, err
	}
	want := hex.EncodeToString(p.BanID[:])
	for _, e := range list {
		if e.BanID == want {
			return e, nil
		}
	}
	return BanEntry{BanID: want, AppID: appID, VersionRange: versionRange, Reason: reason}, nil
}

// SetBanVerdict records the local hub's agree/disagree on a ban.
func (s *Store) SetBanVerdict(banIDHex string, agree bool) error {
	localHub, err := s.LocalHubID()
	if err != nil {
		return err
	}
	idBytes, err := hex.DecodeString(banIDHex)
	if err != nil || len(idBytes) != 16 {
		return fmt.Errorf("datastore: invalid ban id")
	}
	var banID [16]byte
	copy(banID[:], idBytes)
	return banlist.NewStore(s.db).SetVerdict(banlist.Verdict{
		BanID: banID, HubID: localHub, Agree: agree, DecidedAt: time.Now(),
	})
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial Phase 1 schema: all tables from "Storage - SQLite
//           Everywhere" in /plan.md.
//   0.0.3 - migration2: node_registry.pubkey is now nullable. Phase 2's
//           OGM exchange learns of a peer's NodeID (a pubkey hash) long
//           before it learns the actual pubkey bytes, so the column can
//           no longer be NOT NULL.
//   0.0.7 - migration3: hub_registry gains first_seen, last_seen, and
//           status so QuakeMeshMonitor can track backbone hubs like nodes.
//   0.0.19 - migration4: lan_segments and lan_segment_members for
//           Wi-Fi infrastructure view in Monitor.

package storage

// Migrations is the ordered list of schema migrations, applied by
// version number. Do not edit a migration once it has shipped --
// append a new one instead.
var Migrations = []Migration{
	{Version: 1, SQL: migration1},
	{Version: 2, SQL: migration2},
	{Version: 3, SQL: migration3},
	{Version: 4, SQL: migration4},
}

// migration1 creates every table listed in "Storage - SQLite Everywhere"
// in /plan.md. Hub and Android share this schema; Android simply never
// populates hub-only tables (hub_registry, ban_list, ban_verdicts,
// historical_metrics, admin_users).
const migration1 = `
CREATE TABLE node_registry (
	node_id     BLOB PRIMARY KEY,
	pubkey      BLOB NOT NULL,
	first_seen  INTEGER NOT NULL,
	last_seen   INTEGER NOT NULL,
	last_lat    REAL,
	last_lon    REAL,
	status      TEXT NOT NULL
);

CREATE TABLE routing_table (
	destination_node_id BLOB PRIMARY KEY,
	next_hop_node_id     BLOB NOT NULL,
	tq                   REAL NOT NULL,
	latency_ms           INTEGER NOT NULL,
	hop_count            INTEGER NOT NULL
);

CREATE TABLE proximity_events (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	observer_node_id BLOB NOT NULL,
	observed_node_id BLOB NOT NULL,
	rssi             INTEGER NOT NULL,
	transport        TEXT NOT NULL,
	observed_at      INTEGER NOT NULL
);
CREATE INDEX idx_proximity_events_observed ON proximity_events (observed_node_id);

CREATE TABLE trust_endorsements (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	endorser_node_id  BLOB NOT NULL,
	endorsed_node_id  BLOB NOT NULL,
	endorsed_at       INTEGER NOT NULL,
	signature         BLOB NOT NULL,
	UNIQUE (endorser_node_id, endorsed_node_id)
);

CREATE TABLE hub_registry (
	hub_id        BLOB PRIMARY KEY,
	last_ip       TEXT,
	last_port     INTEGER,
	relay_capable INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE relay_hubs (
	hub_id        BLOB PRIMARY KEY,
	ip            TEXT NOT NULL,
	port          INTEGER NOT NULL,
	source        TEXT NOT NULL,
	last_verified INTEGER NOT NULL
);

CREATE TABLE app_presence (
	node_id       BLOB NOT NULL,
	app_id        TEXT NOT NULL,
	app_name      TEXT NOT NULL,
	app_version   TEXT NOT NULL,
	last_reported INTEGER NOT NULL,
	PRIMARY KEY (node_id, app_id)
);

CREATE TABLE ban_list (
	ban_id             BLOB PRIMARY KEY,
	app_id             TEXT NOT NULL,
	version_range      TEXT NOT NULL,
	reason             TEXT NOT NULL,
	proposed_by_hub_id BLOB NOT NULL,
	proposed_at        INTEGER NOT NULL,
	signature          BLOB NOT NULL
);

CREATE TABLE ban_verdicts (
	ban_id      BLOB NOT NULL REFERENCES ban_list (ban_id),
	hub_id      BLOB NOT NULL,
	agree       INTEGER NOT NULL,
	decided_at  INTEGER NOT NULL,
	PRIMARY KEY (ban_id, hub_id)
);

CREATE TABLE dtq_queue (
	bundle_id   BLOB PRIMARY KEY,
	src_node_id BLOB NOT NULL,
	dst_node_id BLOB NOT NULL,
	payload     BLOB NOT NULL,
	expires_at  INTEGER NOT NULL,
	retry_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_dtq_queue_dst ON dtq_queue (dst_node_id);

CREATE TABLE historical_metrics (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	metric_name TEXT NOT NULL,
	node_id     BLOB,
	value       REAL NOT NULL,
	recorded_at INTEGER NOT NULL
);
CREATE INDEX idx_historical_metrics_name_time ON historical_metrics (metric_name, recorded_at);

CREATE TABLE admin_users (
	username             TEXT PRIMARY KEY,
	password_hash        BLOB NOT NULL,
	salt                 BLOB NOT NULL,
	must_change_password INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE config (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// migration2 rebuilds node_registry with a nullable pubkey column.
// SQLite has no ALTER COLUMN, so the standard rebuild pattern is used:
// create the new shape, copy the data across, drop the old table, rename.
const migration2 = `
CREATE TABLE node_registry_v2 (
	node_id     BLOB PRIMARY KEY,
	pubkey      BLOB,
	first_seen  INTEGER NOT NULL,
	last_seen   INTEGER NOT NULL,
	last_lat    REAL,
	last_lon    REAL,
	status      TEXT NOT NULL
);
INSERT INTO node_registry_v2 (node_id, pubkey, first_seen, last_seen, last_lat, last_lon, status)
	SELECT node_id, pubkey, first_seen, last_seen, last_lat, last_lon, status FROM node_registry;
DROP TABLE node_registry;
ALTER TABLE node_registry_v2 RENAME TO node_registry;
`

// migration3 extends hub_registry with liveness fields for Monitor.
const migration3 = `
CREATE TABLE hub_registry_v2 (
	hub_id        BLOB PRIMARY KEY,
	last_ip       TEXT,
	last_port     INTEGER,
	relay_capable INTEGER NOT NULL DEFAULT 0,
	first_seen    INTEGER NOT NULL,
	last_seen     INTEGER NOT NULL,
	status        TEXT NOT NULL
);
INSERT INTO hub_registry_v2 (hub_id, last_ip, last_port, relay_capable, first_seen, last_seen, status)
	SELECT hub_id, last_ip, last_port, relay_capable, 0, 0, 'online' FROM hub_registry;
DROP TABLE hub_registry;
ALTER TABLE hub_registry_v2 RENAME TO hub_registry;
`

// migration4 adds Wi-Fi LAN segment tracking for the infrastructure layer.
const migration4 = `
CREATE TABLE lan_segments (
	segment_id     TEXT PRIMARY KEY,
	gateway_ip     TEXT NOT NULL,
	ssid           TEXT,
	bssid          TEXT,
	first_seen     INTEGER NOT NULL,
	last_seen      INTEGER NOT NULL,
	estimated_lat  REAL,
	estimated_lon  REAL
);

CREATE TABLE lan_segment_members (
	segment_id   TEXT NOT NULL REFERENCES lan_segments (segment_id),
	entity_type  TEXT NOT NULL,
	entity_id    BLOB NOT NULL,
	local_ip     TEXT,
	last_seen    INTEGER NOT NULL,
	PRIMARY KEY (segment_id, entity_type, entity_id)
);
CREATE INDEX idx_lan_segment_members_entity ON lan_segment_members (entity_type, entity_id);
`

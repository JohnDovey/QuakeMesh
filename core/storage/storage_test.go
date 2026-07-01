// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial tests for migrations: applied cleanly, idempotent
//           across reopen, and every documented table exists.

package storage

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_AppliesLatestMigration(t *testing.T) {
	db := openTestDB(t)
	version, err := db.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	want := Migrations[len(Migrations)-1].Version
	if version != want {
		t.Fatalf("schema version = %d, want %d", version, want)
	}
}

func TestOpen_ReopenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open (first): %v", err)
	}
	first.Close()

	second, err := Open(path)
	if err != nil {
		t.Fatalf("Open (second, re-migrate over existing schema): %v", err)
	}
	defer second.Close()

	version, err := second.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	want := Migrations[len(Migrations)-1].Version
	if version != want {
		t.Fatalf("schema version after reopen = %d, want %d", version, want)
	}
}

func TestOpen_AllTablesExist(t *testing.T) {
	db := openTestDB(t)

	wantTables := []string{
		"node_registry",
		"routing_table",
		"proximity_events",
		"trust_endorsements",
		"hub_registry",
		"relay_hubs",
		"app_presence",
		"ban_list",
		"ban_verdicts",
		"dtq_queue",
		"historical_metrics",
		"admin_users",
		"config",
	}

	for _, table := range wantTables {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q: %v", table, err)
		}
	}
}

func TestConfig_InsertAndQuery(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`INSERT INTO config (key, value) VALUES ('ogm_interval_ms', '5000')`); err != nil {
		t.Fatalf("insert config row: %v", err)
	}

	var value string
	if err := db.QueryRow(`SELECT value FROM config WHERE key = ?`, "ogm_interval_ms").Scan(&value); err != nil {
		t.Fatalf("query config row: %v", err)
	}
	if value != "5000" {
		t.Fatalf("value = %q, want %q", value, "5000")
	}
}

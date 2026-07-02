// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial tests for migrations: applied cleanly, idempotent
//           across reopen, and every documented table exists.
//   0.0.3 - Tests for migration2: pubkey accepts NULL, and existing
//           node_registry rows survive the table rebuild. Also a
//           concurrent-writers regression test for the WAL/busy_timeout
//           fix (Hub's OGM engine writes from multiple goroutines).

package storage

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
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
		"lan_segments",
		"lan_segment_members",
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

func TestNodeRegistry_PubkeyAcceptsNull(t *testing.T) {
	db := openTestDB(t)

	_, err := db.Exec(
		`INSERT INTO node_registry (node_id, pubkey, first_seen, last_seen, status) VALUES (?, NULL, 1, 1, 'online')`,
		[]byte("node-a"),
	)
	if err != nil {
		t.Fatalf("insert node_registry row with NULL pubkey: %v", err)
	}

	var pubkey []byte
	err = db.QueryRow(`SELECT pubkey FROM node_registry WHERE node_id = ?`, []byte("node-a")).Scan(&pubkey)
	if err != nil {
		t.Fatalf("query node_registry row: %v", err)
	}
	if pubkey != nil {
		t.Fatalf("pubkey = %v, want nil", pubkey)
	}
}

func TestMigration2_PreservesExistingNodeRegistryRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	// Build a database on schema version 1 only, bypassing the normal
	// migration runner, to simulate a pre-existing installation.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(migration1); err != nil {
		t.Fatalf("apply migration1: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO node_registry (node_id, pubkey, first_seen, last_seen, status) VALUES (?, ?, 1, 2, 'online')`,
		[]byte("node-a"), []byte("pubkey-bytes"),
	); err != nil {
		t.Fatalf("insert v1 node_registry row: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw v1 db: %v", err)
	}

	// Opening through the normal migration runner should now apply
	// migration2 and preserve the row inserted under v1.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrate v1 -> latest): %v", err)
	}
	defer db.Close()

	var pubkey []byte
	var firstSeen, lastSeen int64
	var status string
	err = db.QueryRow(
		`SELECT pubkey, first_seen, last_seen, status FROM node_registry WHERE node_id = ?`, []byte("node-a"),
	).Scan(&pubkey, &firstSeen, &lastSeen, &status)
	if err != nil {
		t.Fatalf("query migrated node_registry row: %v", err)
	}
	if string(pubkey) != "pubkey-bytes" || firstSeen != 1 || lastSeen != 2 || status != "online" {
		t.Fatalf("migrated row = (pubkey=%q, first_seen=%d, last_seen=%d, status=%q), want (pubkey-bytes, 1, 2, online)",
			pubkey, firstSeen, lastSeen, status)
	}
}

func TestOpen_SurvivesConcurrentWriters(t *testing.T) {
	db := openTestDB(t)

	const goroutines = 8
	const writesEach = 50

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*writesEach)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < writesEach; i++ {
				key := []byte(fmt.Sprintf("node-%d-%d", g, i))
				_, err := db.Exec(
					`INSERT INTO node_registry (node_id, first_seen, last_seen, status) VALUES (?, 1, 1, 'online')`,
					key,
				)
				if err != nil {
					errs <- err
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write failed (want WAL + busy_timeout to absorb writer-vs-writer contention): %v", err)
	}
}

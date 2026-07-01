// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.2 - Implemented Open, migration runner (PRAGMA user_version
//           based), and the Phase 1 schema.

// Package storage holds the shared SQLite schema and migrations used by
// both QuakeMeshHub and QuakeMesh Android (modernc.org/sqlite -- pure
// Go, no cgo, so it works under gomobile). See "Storage - SQLite
// Everywhere" in /plan.md for the full table list.
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps a migrated SQLite connection.
type DB struct {
	*sql.DB
}

// Migration is one forward step in the schema history, applied in a
// single transaction and recorded via PRAGMA user_version.
type Migration struct {
	Version int
	SQL     string
}

// Open opens (creating if necessary) the SQLite database at path and
// applies any pending migrations.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", path, err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("storage: enable foreign keys: %w", err)
	}
	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// SchemaVersion returns the database's current schema version.
func (db *DB) SchemaVersion() (int, error) {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("storage: read schema version: %w", err)
	}
	return version, nil
}

func (db *DB) migrate() error {
	current, err := db.SchemaVersion()
	if err != nil {
		return err
	}
	for _, m := range Migrations {
		if m.Version <= current {
			continue
		}
		if err := db.applyMigration(m); err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) applyMigration(m Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("storage: begin migration %d: %w", m.Version, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	if _, err := tx.Exec(m.SQL); err != nil {
		return fmt.Errorf("storage: apply migration %d: %w", m.Version, err)
	}
	// PRAGMA user_version does not accept bound parameters.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.Version)); err != nil {
		return fmt.Errorf("storage: set schema version %d: %w", m.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit migration %d: %w", m.Version, err)
	}
	return nil
}

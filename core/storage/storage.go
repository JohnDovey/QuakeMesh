// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package storage will hold the shared SQLite schema and migrations used
// by both QuakeMeshHub and QuakeMesh Android (modernc.org/sqlite -- pure
// Go, no cgo, so it works under gomobile). See "Storage - SQLite
// Everywhere" in /plan.md for the full table list.
//
// Not yet implemented (Phase 1).
package storage

// Migration is one forward step in the schema history.
type Migration struct {
	Version int
	SQL     string
}

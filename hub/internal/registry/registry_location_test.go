// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.10 - Phase 8 UpdateLocation test.

package registry

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestUpdateLocation(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := New(db)
	var id identity.NodeID
	id[0] = 7
	if _, err := reg.UpsertSeen(id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := reg.UpdateLocation(id, 37.77, -122.42, time.Now()); err != nil {
		t.Fatal(err)
	}
	nodes, err := reg.Nodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("Nodes = %v, %v", nodes, err)
	}
	if nodes[0].LastLat == nil || nodes[0].LastLon == nil {
		t.Fatal("expected location set")
	}
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.7 - hub_registry tests.

package registry

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func testHubID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func TestUpsertHubSeen_MarkHubsStaleBefore(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := New(db)
	id := testHubID(9)
	old := time.Now().Add(-2 * time.Hour)
	changed, err := reg.UpsertHubSeen(id, "127.0.0.1", 47222, old)
	if err != nil || !changed {
		t.Fatalf("UpsertHubSeen = %v, %v", changed, err)
	}
	hubs, err := reg.Hubs()
	if err != nil || len(hubs) != 1 || hubs[0].Status != HubStatusOnline {
		t.Fatalf("Hubs = %+v, %v", hubs, err)
	}

	stale, err := reg.MarkHubsStaleBefore(time.Now().Add(-time.Hour))
	if err != nil || len(stale) != 1 {
		t.Fatalf("MarkHubsStaleBefore past cutoff = %v, %v", stale, err)
	}
	changed, err = reg.UpsertHubSeen(id, "127.0.0.1", 47222, time.Now())
	if err != nil || !changed {
		t.Fatalf("revive UpsertHubSeen = %v, %v", changed, err)
	}
}

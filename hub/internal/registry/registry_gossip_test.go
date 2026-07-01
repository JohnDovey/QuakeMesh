// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9 UpsertGossipRelay test.

package registry

import (
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestRegistry_UpsertGossipRelay(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	reg := New(db)
	sum := sha256.Sum256([]byte("10.0.0.1:99"))
	var id identity.NodeID
	copy(id[:], sum[:])
	ok, err := reg.UpsertGossipRelay(id, "10.0.0.1", 99, time.Now())
	if err != nil || !ok {
		t.Fatalf("UpsertGossipRelay = %v, %v", ok, err)
	}
	relays, err := reg.ListRelayHubs()
	if err != nil {
		t.Fatal(err)
	}
	if len(relays) != 1 || relays[0].Source != "gossip" {
		t.Fatalf("relays = %+v", relays)
	}
}

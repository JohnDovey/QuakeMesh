// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10 apppresence store tests.

package apppresence

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestStore_UpsertAndDiscover(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)

	var node identity.NodeID
	node[0] = 0x01
	now := time.Now()
	if err := store.Upsert(node, "net.example.chat", "Chat", "1.0.0", now); err != nil {
		t.Fatal(err)
	}
	peers, err := store.DiscoverPeers("net.example.chat", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0] != node {
		t.Fatalf("peers = %v", peers)
	}
	stats, err := store.AppStats()
	if err != nil || len(stats) != 1 || stats[0].NodeCount != 1 {
		t.Fatalf("stats = %v, %v", stats, err)
	}
}

func TestVersionMatches(t *testing.T) {
	if !VersionMatches("1.2.3", "") || !VersionMatches("1.2.3", "*") {
		t.Fatal("wildcard should match")
	}
	if VersionMatches("1.2.3", "1.2.4") {
		t.Fatal("exact mismatch")
	}
}

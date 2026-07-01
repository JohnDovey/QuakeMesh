// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial tests: node status transitions, staleness sweep,
//           route upsert/list.

package registry

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db)
}

func nodeID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func TestUpsertSeen_NewNodeReportsChanged(t *testing.T) {
	r := newTestRegistry(t)
	now := time.Now()

	changed, err := r.UpsertSeen(nodeID(1), now)
	if err != nil {
		t.Fatalf("UpsertSeen: %v", err)
	}
	if !changed {
		t.Fatal("first sighting of a node should report statusChanged = true")
	}

	nodes, err := r.Nodes()
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Status != NodeStatusOnline {
		t.Fatalf("Nodes = %+v, want one online node", nodes)
	}
}

func TestUpsertSeen_RefreshDoesNotReportChanged(t *testing.T) {
	r := newTestRegistry(t)
	id := nodeID(1)
	now := time.Now()

	if _, err := r.UpsertSeen(id, now); err != nil {
		t.Fatalf("UpsertSeen (first): %v", err)
	}
	changed, err := r.UpsertSeen(id, now.Add(time.Second))
	if err != nil {
		t.Fatalf("UpsertSeen (refresh): %v", err)
	}
	if changed {
		t.Fatal("refreshing an already-online node should not report statusChanged = true")
	}
}

func TestMarkStaleBefore_ThenUpsertSeenRevivesNode(t *testing.T) {
	r := newTestRegistry(t)
	id := nodeID(1)
	seenAt := time.Now().Add(-time.Hour)

	if _, err := r.UpsertSeen(id, seenAt); err != nil {
		t.Fatalf("UpsertSeen: %v", err)
	}

	stale, err := r.MarkStaleBefore(time.Now())
	if err != nil {
		t.Fatalf("MarkStaleBefore: %v", err)
	}
	if len(stale) != 1 || stale[0] != id {
		t.Fatalf("MarkStaleBefore returned %v, want [%v]", stale, id)
	}

	nodes, err := r.Nodes()
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Status != NodeStatusStale {
		t.Fatalf("Nodes = %+v, want one stale node", nodes)
	}

	changed, err := r.UpsertSeen(id, time.Now())
	if err != nil {
		t.Fatalf("UpsertSeen (revive): %v", err)
	}
	if !changed {
		t.Fatal("reviving a stale node should report statusChanged = true")
	}
}

func TestMarkStaleBefore_LeavesRecentNodesOnline(t *testing.T) {
	r := newTestRegistry(t)
	id := nodeID(1)

	if _, err := r.UpsertSeen(id, time.Now()); err != nil {
		t.Fatalf("UpsertSeen: %v", err)
	}

	stale, err := r.MarkStaleBefore(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("MarkStaleBefore: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("MarkStaleBefore returned %v, want none", stale)
	}
}

func TestUpsertRoute_InsertAndUpdate(t *testing.T) {
	r := newTestRegistry(t)
	dst, nextHop := nodeID(1), nodeID(2)

	if err := r.UpsertRoute(Route{Destination: dst, NextHop: nextHop, TQ: 1.0, LatencyMs: 0, HopCount: 1}); err != nil {
		t.Fatalf("UpsertRoute (insert): %v", err)
	}

	routes, err := r.Routes()
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	if len(routes) != 1 || routes[0].HopCount != 1 {
		t.Fatalf("Routes = %+v, want one route with hop_count 1", routes)
	}

	// Update the same destination with a better route.
	newNextHop := nodeID(3)
	if err := r.UpsertRoute(Route{Destination: dst, NextHop: newNextHop, TQ: 0.9, LatencyMs: 20, HopCount: 2}); err != nil {
		t.Fatalf("UpsertRoute (update): %v", err)
	}

	routes, err = r.Routes()
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("Routes = %+v, want exactly one row for the destination", routes)
	}
	if routes[0].NextHop != newNextHop || routes[0].HopCount != 2 {
		t.Fatalf("Routes[0] = %+v, want next hop %v hop_count 2", routes[0], newNextHop)
	}
}

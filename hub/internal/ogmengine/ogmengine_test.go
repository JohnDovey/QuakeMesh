// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial tests: direct peer convergence, event notification,
//           stale timeout, and own-broadcast rejection.

package ogmengine

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

// recordingHandler collects the events an Engine reports, for tests to
// assert against.
type recordingHandler struct {
	mu            sync.Mutex
	statusChanges []registry.NodeStatus
	routesChanged []registry.Route
}

func (h *recordingHandler) NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusChanges = append(h.statusChanges, status)
}

func (h *recordingHandler) RouteChanged(route registry.Route) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.routesChanged = append(h.routesChanged, route)
}

func (h *recordingHandler) routeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.routesChanged)
}

func newTestRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return registry.New(db)
}

func testNodeID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met before timeout")
	}
}

func TestEngine_DirectPeerConvergence(t *testing.T) {
	regA, regB := newTestRegistry(t), newTestRegistry(t)
	idA, idB := testNodeID(1), testNodeID(2)

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19011", Peers: []string{"127.0.0.1:19012"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: regA,
	})
	engB := New(Config{
		SelfID: idB, BindAddr: "127.0.0.1:19012", Peers: []string{"127.0.0.1:19011"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: regB,
	})

	if err := engA.Start(); err != nil {
		t.Fatalf("engA.Start: %v", err)
	}
	defer engA.Close()
	if err := engB.Start(); err != nil {
		t.Fatalf("engB.Start: %v", err)
	}
	defer engB.Close()

	waitFor(t, 2*time.Second, func() bool {
		routes, err := regA.Routes()
		return err == nil && len(routes) == 1 && routes[0].Destination == idB && routes[0].HopCount == 1
	})
	waitFor(t, 2*time.Second, func() bool {
		routes, err := regB.Routes()
		return err == nil && len(routes) == 1 && routes[0].Destination == idA && routes[0].HopCount == 1
	})

	nodesA, err := regA.Nodes()
	if err != nil {
		t.Fatalf("regA.Nodes: %v", err)
	}
	if len(nodesA) != 1 || nodesA[0].NodeID != idB || nodesA[0].Status != registry.NodeStatusOnline {
		t.Fatalf("regA.Nodes = %+v, want one online node with id %v", nodesA, idB)
	}
}

func TestEngine_NotifiesHandlerOnNewNodeAndRoute(t *testing.T) {
	regA, regB := newTestRegistry(t), newTestRegistry(t)
	idA, idB := testNodeID(1), testNodeID(2)
	handlerA := &recordingHandler{}

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19021", Peers: []string{"127.0.0.1:19022"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: regA, Handler: handlerA,
	})
	engB := New(Config{
		SelfID: idB, BindAddr: "127.0.0.1:19022", Peers: []string{"127.0.0.1:19021"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: regB,
	})

	if err := engA.Start(); err != nil {
		t.Fatalf("engA.Start: %v", err)
	}
	defer engA.Close()
	if err := engB.Start(); err != nil {
		t.Fatalf("engB.Start: %v", err)
	}
	defer engB.Close()

	waitFor(t, 2*time.Second, func() bool { return handlerA.routeCount() > 0 })

	handlerA.mu.Lock()
	defer handlerA.mu.Unlock()
	if len(handlerA.statusChanges) != 1 || handlerA.statusChanges[0] != registry.NodeStatusOnline {
		t.Fatalf("statusChanges = %v, want exactly one NodeStatusOnline", handlerA.statusChanges)
	}
	if handlerA.routesChanged[0].Destination != idB {
		t.Fatalf("routesChanged[0].Destination = %v, want %v", handlerA.routesChanged[0].Destination, idB)
	}
}

func TestEngine_MarksPeerStaleAfterTimeout(t *testing.T) {
	regA, regB := newTestRegistry(t), newTestRegistry(t)
	idA, idB := testNodeID(1), testNodeID(2)
	handlerA := &recordingHandler{}

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19031", Peers: []string{"127.0.0.1:19032"},
		Interval: 20 * time.Millisecond, StaleAfter: 150 * time.Millisecond, TTL: 3, Registry: regA, Handler: handlerA,
	})
	engB := New(Config{
		SelfID: idB, BindAddr: "127.0.0.1:19032", Peers: []string{"127.0.0.1:19031"},
		Interval: 20 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: regB,
	})

	if err := engA.Start(); err != nil {
		t.Fatalf("engA.Start: %v", err)
	}
	defer engA.Close()
	if err := engB.Start(); err != nil {
		t.Fatalf("engB.Start: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		nodes, err := regA.Nodes()
		return err == nil && len(nodes) == 1 && nodes[0].Status == registry.NodeStatusOnline
	})

	// Stop B: it stops sending OGMs, so A's stale sweep should mark it
	// stale once StaleAfter has elapsed with nothing heard.
	if err := engB.Close(); err != nil {
		t.Fatalf("engB.Close: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		nodes, err := regA.Nodes()
		return err == nil && len(nodes) == 1 && nodes[0].Status == registry.NodeStatusStale
	})
}

func TestEngine_IgnoresOwnBroadcast(t *testing.T) {
	reg := newTestRegistry(t)
	id := testNodeID(1)

	// Misconfigured (or self-referential) peer list: this engine sends
	// its own OGMs to itself.
	eng := New(Config{
		SelfID: id, BindAddr: "127.0.0.1:19041", Peers: []string{"127.0.0.1:19041"},
		Interval: 20 * time.Millisecond, StaleAfter: time.Hour, TTL: 3, Registry: reg,
	})
	if err := eng.Start(); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}
	defer eng.Close()

	time.Sleep(200 * time.Millisecond)

	nodes, err := reg.Nodes()
	if err != nil {
		t.Fatalf("reg.Nodes: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("Nodes = %+v, want none: engine should never register itself", nodes)
	}
}

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial tests: direct peer convergence, event notification,
//           stale timeout, and own-broadcast rejection.
//   0.0.6 - Phase 5: three-hop chain and failover tests.

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
}

func TestEngine_MarksPeerStaleAfterTimeout(t *testing.T) {
	regA, regB := newTestRegistry(t), newTestRegistry(t)
	idA, idB := testNodeID(1), testNodeID(2)

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19031", Peers: []string{"127.0.0.1:19032"},
		Interval: 20 * time.Millisecond, StaleAfter: 150 * time.Millisecond, TTL: 3, Registry: regA,
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
		t.Fatalf("Nodes = %+v, want none", nodes)
	}
}

func TestEngine_ThreeHopChain(t *testing.T) {
	regA, regB, regC := newTestRegistry(t), newTestRegistry(t), newTestRegistry(t)
	idA, idB, idC := testNodeID(1), testNodeID(2), testNodeID(3)

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19051", Peers: []string{"127.0.0.1:19052"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 4, Registry: regA,
	})
	engB := New(Config{
		SelfID: idB, BindAddr: "127.0.0.1:19052", Peers: []string{"127.0.0.1:19051", "127.0.0.1:19053"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 4, Registry: regB,
	})
	engC := New(Config{
		SelfID: idC, BindAddr: "127.0.0.1:19053", Peers: []string{"127.0.0.1:19052"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 4, Registry: regC,
	})

	for _, eng := range []*Engine{engA, engB, engC} {
		if err := eng.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		defer eng.Close()
	}

	waitFor(t, 4*time.Second, func() bool {
		route, ok, err := regA.GetRoute(idC)
		return err == nil && ok && route.NextHop == idB && route.HopCount == 2
	})
}

func TestEngine_FailoverToAlternateOnStaleNextHop(t *testing.T) {
	regA, regB, regC := newTestRegistry(t), newTestRegistry(t), newTestRegistry(t)
	idA, idB, idC := testNodeID(11), testNodeID(12), testNodeID(13)

	engA := New(Config{
		SelfID: idA, BindAddr: "127.0.0.1:19061", Peers: []string{"127.0.0.1:19062", "127.0.0.1:19063"},
		Interval: 30 * time.Millisecond, StaleAfter: 200 * time.Millisecond, TTL: 4, Registry: regA,
	})
	engB := New(Config{
		SelfID: idB, BindAddr: "127.0.0.1:19062", Peers: []string{"127.0.0.1:19061", "127.0.0.1:19063"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 4, Registry: regB,
	})
	engC := New(Config{
		SelfID: idC, BindAddr: "127.0.0.1:19063", Peers: []string{"127.0.0.1:19061", "127.0.0.1:19062"},
		Interval: 30 * time.Millisecond, StaleAfter: time.Hour, TTL: 4, Registry: regC,
	})

	if err := engA.Start(); err != nil {
		t.Fatal(err)
	}
	defer engA.Close()
	if err := engB.Start(); err != nil {
		t.Fatal(err)
	}
	defer engB.Close()
	if err := engC.Start(); err != nil {
		t.Fatal(err)
	}
	defer engC.Close()

	waitFor(t, 3*time.Second, func() bool {
		routes, err := regA.Routes()
		return err == nil && len(routes) >= 2
	})

	engB.Close()
	waitFor(t, 3*time.Second, func() bool {
		routes, err := regA.Routes()
		if err != nil {
			return false
		}
		for _, r := range routes {
			if r.NextHop == idB {
				return false
			}
		}
		return len(routes) >= 1
	})
}

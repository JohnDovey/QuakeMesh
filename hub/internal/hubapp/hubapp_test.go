// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Integration test: 3 in-process hub instances on distinct
//           localhost ports converge their OGM-based routing tables.
//   0.0.6 - Phase 5: linear three-hub chain reaches hop_count 2.
//   0.0.7 - Phase 6: DTN queue drains when route to destination appears.

package hubapp

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

func newTestConfig(t *testing.T, ogmPort, mgmtPort int, peerPorts []int) Config {
	t.Helper()
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.IdentityPath = filepath.Join(dir, "identity.seed")
	cfg.DBPath = filepath.Join(dir, "hub.db")
	cfg.OGMBindAddr = fmt.Sprintf("127.0.0.1:%d", ogmPort)
	cfg.ManagementAddr = fmt.Sprintf("127.0.0.1:%d", mgmtPort)
	cfg.OGMInterval = 30 * time.Millisecond
	cfg.StaleAfter = time.Hour
	for _, p := range peerPorts {
		cfg.Peers = append(cfg.Peers, fmt.Sprintf("127.0.0.1:%d", p))
	}
	return cfg
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met before timeout")
	}
}

func TestHub_ThreeHubsConvergeRouting(t *testing.T) {
	ogmPorts := []int{19101, 19102, 19103}
	mgmtPorts := []int{19111, 19112, 19113}

	var hubs []*Hub
	for i := range ogmPorts {
		var peers []int
		for j, p := range ogmPorts {
			if j != i {
				peers = append(peers, p)
			}
		}
		cfg := newTestConfig(t, ogmPorts[i], mgmtPorts[i], peers)
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New (hub %d): %v", i, err)
		}
		if err := h.Start(); err != nil {
			t.Fatalf("Start (hub %d): %v", i, err)
		}
		t.Cleanup(func() { h.Close() })
		hubs = append(hubs, h)
	}

	for i, h := range hubs {
		waitFor(t, 3*time.Second, func() bool {
			routes, err := h.Registry.Routes()
			return err == nil && len(routes) == 2
		})

		routes, err := h.Registry.Routes()
		if err != nil {
			t.Fatalf("hub %d Routes: %v", i, err)
		}
		if len(routes) != 2 {
			t.Fatalf("hub %d has %d routes, want 2 (one per other hub)", i, len(routes))
		}
		for _, r := range routes {
			if r.Destination == h.Identity.NodeID {
				t.Errorf("hub %d has a route to its own NodeID: %v", i, r.Destination)
			}
		}

		nodes, err := h.Registry.Nodes()
		if err != nil {
			t.Fatalf("hub %d Nodes: %v", i, err)
		}
		if len(nodes) != 2 {
			t.Fatalf("hub %d has %d known nodes, want 2", i, len(nodes))
		}
	}
}

func TestHub_LinearThreeHopChain(t *testing.T) {
	ogmPorts := []int{19201, 19202, 19203}
	mgmtPorts := []int{19211, 19212, 19213}
	peerMap := [][]int{{19202}, {19201, 19203}, {19202}}

	var hubs []*Hub
	for i := range ogmPorts {
		cfg := newTestConfig(t, ogmPorts[i], mgmtPorts[i], peerMap[i])
		cfg.OGMTTL = 4
		h, err := New(cfg)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := h.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		t.Cleanup(func() { h.Close() })
		hubs = append(hubs, h)
	}

	waitFor(t, 5*time.Second, func() bool {
		routes, err := hubs[0].Registry.Routes()
		if err != nil {
			return false
		}
		for _, r := range routes {
			if r.HopCount == 2 {
				return true
			}
		}
		return false
	})
}

func TestHub_DTNQueueDrainsOnRoute(t *testing.T) {
	cfg := newTestConfig(t, 19301, 19311, nil)
	cfg.DTNInterval = 50 * time.Millisecond
	h, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })

	var dst identity.NodeID
	dst[0] = 0x42
	if err := h.DTN.Enqueue(h.Identity.NodeID, dst, []byte("pending")); err != nil {
		t.Fatal(err)
	}
	depth, err := dtn.NewStore(h.DB).Depth()
	if err != nil || depth != 1 {
		t.Fatalf("initial depth = %d, %v; want 1", depth, err)
	}

	if _, err := h.Registry.UpsertSeen(dst, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := h.Registry.UpsertRoute(registry.Route{
		Destination: dst,
		NextHop:     h.Identity.NodeID,
		TQ:          0.9,
		HopCount:    1,
	}); err != nil {
		t.Fatal(err)
	}
	h.DTN.OnRouteChanged(registry.Route{Destination: dst})

	waitFor(t, 2*time.Second, func() bool {
		depth, err := dtn.NewStore(h.DB).Depth()
		return err == nil && depth == 0
	})
}

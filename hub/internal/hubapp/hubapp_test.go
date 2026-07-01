// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Integration test: 3 in-process hub instances on distinct
//           localhost ports converge their OGM-based routing tables.
//           See "Verification Strategy" /hub in /plan.md -- this
//           exercises 3 independent Hub instances, each with its own
//           UDP socket, SQLite file, and identity, communicating only
//           over real network sockets on localhost. It stops short of
//           spawning 3 separate OS processes (unnecessary complexity
//           for what this phase needs to prove) but is otherwise the
//           same test.

package hubapp

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
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
			if r.HopCount != 1 {
				t.Errorf("hub %d route to %v has hop_count %d, want 1 (Phase 2 is direct-only)", i, r.Destination, r.HopCount)
			}
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

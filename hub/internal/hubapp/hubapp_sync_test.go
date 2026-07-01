// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9 gossip relay propagation test.

package hubapp

import (
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
)

func relayHubID(ip string, port int) identity.NodeID {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", ip, port)))
	var id identity.NodeID
	copy(id[:], sum[:])
	return id
}

func TestHub_GossipPropagatesRelayHub(t *testing.T) {
	// OGM ports must leave room for sync bind (OGM port + 1) without overlap.
	ogmPorts := []int{20401, 20403}
	mgmtPorts := []int{20411, 20413}

	var hubs []*Hub
	for i := range ogmPorts {
		cfg := newTestConfig(t, ogmPorts[i], mgmtPorts[i], nil)
		if i == 0 {
			cfg.Peers = []string{fmt.Sprintf("127.0.0.1:%d", ogmPorts[1])}
		} else {
			cfg.Peers = []string{fmt.Sprintf("127.0.0.1:%d", ogmPorts[0])}
		}
		cfg.SyncInterval = 50 * time.Millisecond
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

	hubID := relayHubID("203.0.113.50", 47222)
	now := time.Now()
	if _, err := hubs[0].Registry.UpsertGossipRelay(hubID, "203.0.113.50", 47222, now); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 5*time.Second, func() bool {
		relays, err := hubs[1].Registry.ListRelayHubs()
		if err != nil {
			return false
		}
		for _, r := range relays {
			if r.HubID == hubID && r.IP == "203.0.113.50" && r.Port == 47222 {
				return true
			}
		}
		return false
	})
}

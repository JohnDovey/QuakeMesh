// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.13 - Phase 11 ban proposal gossip propagation test.

package hubapp

import (
	"fmt"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/banlist"
)

func TestHub_GossipPropagatesBanProposal(t *testing.T) {
	ogmPorts := []int{20501, 20503}
	mgmtPorts := []int{20511, 20513}

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

	store := banlist.NewStore(hubs[0].DB)
	now := time.Now()
	p := banlist.Proposal{
		AppID: "net.evil.app", VersionRange: "*", Reason: "test",
		ProposedBy: hubs[0].Identity.NodeID, ProposedAt: now,
	}
	if err := store.Propose(p); err != nil {
		t.Fatal(err)
	}
	unsigned, err := store.ListUnsignedByProposer(hubs[0].Identity.NodeID)
	if err != nil || len(unsigned) == 0 {
		t.Fatalf("unsigned: %v, %v", unsigned, err)
	}
	p = unsigned[0]
	if err := store.UpdateSignature(p.BanID, hubs[0].Identity.Sign(banlist.SignBytes(p))); err != nil {
		t.Fatal(err)
	}

	waitFor(t, 5*time.Second, func() bool {
		remote, err := banlist.NewStore(hubs[1].DB).ListProposals()
		if err != nil {
			return false
		}
		for _, pr := range remote {
			if pr.AppID == "net.evil.app" {
				return true
			}
		}
		return false
	})
}

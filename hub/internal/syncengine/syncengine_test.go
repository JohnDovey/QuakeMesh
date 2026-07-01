// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9 syncengine port mapping tests.

package syncengine

import "testing"

func TestPeerSyncAddrs_incrementsPort(t *testing.T) {
	got := PeerSyncAddrs([]string{"127.0.0.1:47222", "10.0.0.2:9000"})
	want := []string{"127.0.0.1:47223", "10.0.0.2:9001"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("peer[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSyncBindAddr(t *testing.T) {
	got, err := SyncBindAddr("0.0.0.0:47222")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.0.0.0:47223" {
		t.Fatalf("SyncBindAddr = %q, want 0.0.0.0:47223", got)
	}
}

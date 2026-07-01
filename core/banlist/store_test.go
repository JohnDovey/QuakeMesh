// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.13 - Phase 11 banlist store tests.

package banlist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestStore_ProposeVerdictEnforce(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)

	var hub, other identity.NodeID
	hub[1] = 1
	other[2] = 2
	p := Proposal{
		AppID: "net.evil.app", VersionRange: "1.*", Reason: "malware",
		ProposedBy: hub, ProposedAt: time.Now(), Signature: []byte("sig"),
	}
	if err := store.Propose(p); err != nil {
		t.Fatal(err)
	}
	if err := store.SetVerdict(Verdict{BanID: p.BanID, HubID: hub, Agree: true, DecidedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	enforced, err := store.IsLocallyEnforced(hub, "net.evil.app", "1.2.3")
	if err != nil || !enforced {
		t.Fatalf("enforced = %v, %v", enforced, err)
	}
	enforced, err = store.IsLocallyEnforced(other, "net.evil.app", "1.2.3")
	if err != nil || enforced {
		t.Fatalf("other hub should not enforce locally")
	}
	tally, err := store.TallyVerdicts(p.BanID)
	if err != nil || tally.Agree != 1 {
		t.Fatalf("tally = %+v", tally)
	}
}

func TestVersionInRange(t *testing.T) {
	if !VersionInRange("1.2.3", "1.*") || VersionInRange("2.0.0", "1.*") {
		t.Fatal("prefix range")
	}
	if !VersionInRange("9.9.9", "*") {
		t.Fatal("wildcard")
	}
}

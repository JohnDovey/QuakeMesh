// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.9 - Phase 7 trust store tests.

package trust

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func testID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "trust.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestRecordProximity_EndorsementRequiresProximity(t *testing.T) {
	s := testStore(t)
	observer, observed := testID(1), testID(2)
	now := time.Now()

	if err := s.AddEndorsement(observer, observed, []byte("sig"), now); err == nil {
		t.Fatal("expected endorsement without proximity to fail")
	}
	if err := s.RecordProximity(observer, observed, -55, TransportHubDirect, now); err != nil {
		t.Fatal(err)
	}
	if err := s.AddEndorsement(observer, observed, []byte("sig"), now); err != nil {
		t.Fatal(err)
	}
	n, err := s.EndorsementCount(observed)
	if err != nil || n != 1 {
		t.Fatalf("EndorsementCount = %d, %v", n, err)
	}
}

func TestScoreForNode(t *testing.T) {
	s := testStore(t)
	observed := testID(3)
	observer := testID(4)
	now := time.Now()
	first := now.Add(-48 * time.Hour)
	_ = s.RecordProximity(observer, observed, -55, TransportHubDirect, now)
	b, err := s.ScoreForNode(observed, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if b.Total <= 0 || b.ProximityComponent <= 0 {
		t.Fatalf("expected positive score, got %+v", b)
	}
}

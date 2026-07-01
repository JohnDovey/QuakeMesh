// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.7 - Phase 6 DTN store tests.

package dtn

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "dtn.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func testID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func TestEnqueue_List_Delete(t *testing.T) {
	s := testStore(t)
	src, dst := testID(1), testID(2)
	b, err := s.Enqueue(src, dst, []byte("payload"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}
	if err := s.Delete(b.BundleID); err != nil {
		t.Fatal(err)
	}
	depth, err := s.Depth()
	if err != nil || depth != 0 {
		t.Fatalf("Depth = %d, %v", depth, err)
	}
}

func TestExpireBefore(t *testing.T) {
	s := testStore(t)
	_, err := s.Enqueue(testID(1), testID(2), []byte("x"), time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	n, err := s.ExpireBefore(time.Now())
	if err != nil || n != 1 {
		t.Fatalf("ExpireBefore = %d, %v", n, err)
	}
}

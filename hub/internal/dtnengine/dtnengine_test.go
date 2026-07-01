// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.7 - Phase 6 DTN engine tests.

package dtnengine

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/dtn"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

type depthSpy struct {
	mu     sync.Mutex
	depths []int
}

func (s *depthSpy) DtnQueueDepthChanged(depth int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.depths = append(s.depths, depth)
}

func (s *depthSpy) last() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.depths) == 0 {
		return -1
	}
	return s.depths[len(s.depths)-1]
}

func testID(b byte) identity.NodeID {
	var id identity.NodeID
	id[0] = b
	return id
}

func TestEngine_DeliversWhenRouteAppears(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := registry.New(db)
	store := dtn.NewStore(db)
	spy := &depthSpy{}
	src, dst := testID(1), testID(2)

	eng := New(Config{
		Store:    store,
		Registry: reg,
		Handler:  spy,
		TTL:      time.Hour,
		Interval: time.Hour,
	})
	eng.Start()
	t.Cleanup(eng.Close)

	if err := eng.Enqueue(src, dst, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if spy.last() != 1 {
		t.Fatalf("depth after enqueue = %d, want 1", spy.last())
	}

	eng.processOnce()
	if spy.last() != 1 {
		t.Fatalf("depth without route = %d, want 1", spy.last())
	}

	if _, err := reg.UpsertSeen(dst, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := reg.UpsertRoute(registry.Route{
		Destination: dst,
		NextHop:     testID(3),
		TQ:          0.9,
		HopCount:    1,
	}); err != nil {
		t.Fatal(err)
	}
	eng.OnRouteChanged(registry.Route{Destination: dst})

	depth, err := store.Depth()
	if err != nil || depth != 0 {
		t.Fatalf("Depth = %d, %v; want 0", depth, err)
	}
	if spy.last() != 0 {
		t.Fatalf("notified depth = %d, want 0", spy.last())
	}
}

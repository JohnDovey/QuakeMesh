// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.11 - Phase 9 metrics store tests.

package metrics

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestRecord_Query(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := NewStore(db)
	var id identity.NodeID
	id[0] = 1
	now := time.Now()
	if err := s.Record(MetricRouteLatencyMs, &id, 42, now); err != nil {
		t.Fatal(err)
	}
	samples, err := s.Query(MetricRouteLatencyMs, now.Add(-time.Hour), now.Add(time.Hour), 10)
	if err != nil || len(samples) != 1 || samples[0].Value != 42 {
		t.Fatalf("samples = %+v, %v", samples, err)
	}
}

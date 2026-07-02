// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - LAN segment store tests.

package lansegments

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lancontext"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestRecordMembership_UpsertsSegmentAndMembers(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var nodeID identity.NodeID
	nodeID[3] = 0x33
	now := time.Now()
	ctx := lancontext.Context{
		GatewayIP: "192.168.0.1",
		LocalIP:   "192.168.0.42",
		SSID:      "QuakeMesh",
		BSSID:     "aa:bb:cc:dd:ee:01",
	}
	store := NewStore(db)
	if err := store.RecordMembership(EntityNode, nodeID, ctx, now); err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(
		`INSERT INTO node_registry (node_id, first_seen, last_seen, last_lat, last_lon, status)
		 VALUES (?, ?, ?, ?, ?, 'online')`,
		nodeID[:], now.UnixMilli(), now.UnixMilli(), -36.85, 174.76,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordMembership(EntityNode, nodeID, ctx, now); err != nil {
		t.Fatal(err)
	}

	segments, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 {
		t.Fatalf("segments = %d", len(segments))
	}
	seg := segments[0]
	if seg.GatewayIP != ctx.GatewayIP || seg.SSID != ctx.SSID || seg.BSSID != ctx.BSSID {
		t.Fatalf("segment = %+v", seg)
	}
	if len(seg.NodeIDs) != 1 || seg.NodeIDs[0] != nodeID.String() {
		t.Fatalf("node ids = %v", seg.NodeIDs)
	}
	if seg.EstimatedLat == nil || seg.EstimatedLon == nil {
		t.Fatalf("expected estimated position, got %+v", seg)
	}
}

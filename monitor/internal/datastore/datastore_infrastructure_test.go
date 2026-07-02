// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.19 - infrastructure API tests.

package datastore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lancontext"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

func TestInfrastructure_ReturnsSegmentsWithMembers(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var nodeID identity.NodeID
	nodeID[4] = 0x44
	now := time.Now()
	ctx := lancontext.Context{GatewayIP: "192.168.88.1", SSID: "MeshWiFi"}
	if err := lansegments.NewStore(db).RecordMembership(lansegments.EntityNode, nodeID, ctx, now); err != nil {
		t.Fatal(err)
	}

	segments, err := New(db).Infrastructure()
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 1 {
		t.Fatalf("segments = %d", len(segments))
	}
	if segments[0].GatewayIP != "192.168.88.1" || segments[0].MemberCount != 1 {
		t.Fatalf("segment = %+v", segments[0])
	}
}

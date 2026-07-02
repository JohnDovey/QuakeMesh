// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN discovery registers nodes from multicast beacons.

package landiscovery

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lanbeacon"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

type recordingNotifier struct {
	node identity.NodeID
}

func (r *recordingNotifier) NodeStatusChanged(nodeID identity.NodeID, status registry.NodeStatus) {
	if status == registry.NodeStatusOnline {
		r.node = nodeID
	}
}

func TestEngine_NodeBeaconRegistersNode(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := registry.New(db)
	notifier := &recordingNotifier{}
	var hubID identity.NodeID
	hubID[1] = 0x11

	eng := New(Config{
		BindAddr:      "0.0.0.0:0",
		HubNodeID:     hubID,
		HeartbeatPort: 18085,
		OGMPort:       47222,
		Interval:      time.Hour,
		Registry:      reg,
		Notifier:      notifier,
	})
	if err := eng.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { eng.Close() })

	var nodeID identity.NodeID
	nodeID[2] = 0x22
	lat, lon := -36.85, 174.76
	payload, err := lanbeacon.NodeBeacon(nodeID.String(), &lat, &lon, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	addr, ok := eng.conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		t.Fatal("unexpected local addr")
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for notifier.node != nodeID && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if notifier.node != nodeID {
		t.Fatalf("notifier node = %v", notifier.node)
	}
}

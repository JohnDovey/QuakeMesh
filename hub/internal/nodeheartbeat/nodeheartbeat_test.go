// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.17 - node heartbeat tests.

package nodeheartbeat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
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

func TestServer_HeartbeatRegistersNode(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := registry.New(db)
	notifier := &recordingNotifier{}
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Registry:   reg,
		Notifier:   notifier,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })

	var nodeID identity.NodeID
	nodeID[7] = 0xab
	lat, lon := -36.85, 174.76
	body, _ := json.Marshal(map[string]any{
		"node_id": nodeID.String(),
		"lat":     lat,
		"lon":     lon,
	})
	base := "http://" + srv.Addr().String()
	rec, err := http.Post(base+"/v1/heartbeat", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rec.Body.Close()
	if rec.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", rec.StatusCode)
	}
	if notifier.node != nodeID {
		t.Fatalf("notifier node = %v", notifier.node)
	}
	nodes, err := reg.Nodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("nodes = %v, %v", nodes, err)
	}
	if nodes[0].LastLat == nil || *nodes[0].LastLat != lat {
		t.Fatalf("lat = %v", nodes[0].LastLat)
	}
	_ = time.Now()
}

type recordingSOSNotifier struct {
	node    identity.NodeID
	appID   string
	topic   string
	payload []byte
}

func (r *recordingSOSNotifier) SosAlertPublished(nodeID identity.NodeID, appID, topic string, payload []byte) {
	r.node = nodeID
	r.appID = appID
	r.topic = topic
	r.payload = append([]byte(nil), payload...)
}

func TestServer_SOSNotifiesMonitor(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := registry.New(db)
	sosNotifier := &recordingSOSNotifier{}
	srv := New(Config{
		ListenAddr:  "127.0.0.1:0",
		Registry:    reg,
		Notifier:    &recordingNotifier{},
		SOSNotifier: sosNotifier,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })

	var nodeID identity.NodeID
	nodeID[9] = 0xcd
	body, _ := json.Marshal(map[string]any{
		"node_id": nodeID.String(),
		"text":    "injured, need help",
		"lat":     -36.85,
		"lon":     174.76,
		"sent_at": time.Now().UnixMilli(),
	})
	base := "http://" + srv.Addr().String()
	rec, err := http.Post(base+"/v1/sos", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	rec.Body.Close()
	if rec.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", rec.StatusCode)
	}
	if sosNotifier.node != nodeID || sosNotifier.topic != "sos" {
		t.Fatalf("sos notifier = %+v", sosNotifier)
	}
}

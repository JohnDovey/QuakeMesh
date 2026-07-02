// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.17 - node heartbeat tests.
//   0.0.19 - heartbeat lan_context upserts infrastructure segment.

package nodeheartbeat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/lansegments"
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

func TestServer_HeartbeatLanContextUpsertsSegment(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := registry.New(db)
	segments := lansegments.NewStore(db)
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Registry:   reg,
		Notifier:   &recordingNotifier{},
		Segments:   segments,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })

	var nodeID identity.NodeID
	nodeID[8] = 0xef
	body, _ := json.Marshal(map[string]any{
		"node_id": nodeID.String(),
		"lan_context": map[string]string{
			"gateway_ip": "10.1.1.1",
			"local_ip":   "10.1.1.50",
			"ssid":       "CampNet",
			"bssid":      "de:ad:be:ef:00:01",
		},
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
	list, err := segments.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("segments = %d", len(list))
	}
	if list[0].GatewayIP != "10.1.1.1" || list[0].SSID != "CampNet" {
		t.Fatalf("segment = %+v", list[0])
	}
	if len(list[0].NodeIDs) != 1 || list[0].NodeIDs[0] != nodeID.String() {
		t.Fatalf("members = %+v", list[0].NodeIDs)
	}
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

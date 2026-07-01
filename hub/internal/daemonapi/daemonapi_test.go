// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.12 - Phase 10 daemonapi tests.

package daemonapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

type fakeSender struct {
	lastDst identity.NodeID
}

func (f *fakeSender) Enqueue(src, dst identity.NodeID, payload []byte) error {
	f.lastDst = dst
	return nil
}

func TestServer_RegisterSendReceive(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var self identity.NodeID
	self[1] = 0xab
	sender := &fakeSender{}
	srv := New(Config{
		SelfID:     self,
		ListenAddr: "tcp:127.0.0.1:0",
		Apps:       apppresence.NewStore(db),
		Sender:     sender,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	base := "http://" + srv.Addr().String()

	regBody, _ := json.Marshal(map[string]string{
		"app_id": "net.test.app", "app_name": "Test", "app_version": "1.0.0",
	})
	rec, err := http.Post(base+"/v1/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Body.Close()
	var regResp struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&regResp); err != nil || regResp.SessionToken == "" {
		t.Fatalf("register: %v", err)
	}

	var dst identity.NodeID
	dst[2] = 0xcd
	sendBody, _ := json.Marshal(map[string]string{
		"dest_node_id": dst.String(),
		"payload_b64":  base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/send", bytes.NewReader(sendBody))
	req.Header.Set("X-Mesh-Session", regResp.SessionToken)
	rec2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rec2.Body.Close()
	if sender.lastDst != dst {
		t.Fatalf("dst = %v, want %v", sender.lastDst, dst)
	}

	_ = srv.DeliverLocal(dst, self, []byte("inbound"))
	req3, _ := http.NewRequest(http.MethodGet, base+"/v1/receive", nil)
	req3.Header.Set("X-Mesh-Session", regResp.SessionToken)
	rec3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer rec3.Body.Close()
	var recv struct {
		PayloadB64 string `json:"payload_b64"`
	}
	if err := json.NewDecoder(rec3.Body).Decode(&recv); err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(recv.PayloadB64)
	if string(raw) != "inbound" {
		t.Fatalf("payload = %q", raw)
	}
}

func TestServer_DiscoverPeers(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	apps := apppresence.NewStore(db)
	var peer identity.NodeID
	peer[3] = 0xef
	if err := apps.Upsert(peer, "net.example.chat", "Chat", "2.0.0", time.Now()); err != nil {
		t.Fatal(err)
	}
	var self identity.NodeID
	srv := New(Config{SelfID: self, ListenAddr: "tcp:127.0.0.1:0", Apps: apps})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	base := "http://" + srv.Addr().String()

	regBody, _ := json.Marshal(map[string]string{"app_id": "net.test.app", "app_version": "1.0.0"})
	rec, _ := http.Post(base+"/v1/register", "application/json", bytes.NewReader(regBody))
	var regResp struct {
		SessionToken string `json:"session_token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&regResp)
	rec.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, base+"/v1/discover-peers?app_id=net.example.chat", nil)
	req.Header.Set("X-Mesh-Session", regResp.SessionToken)
	rec2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rec2.Body.Close()
	var out struct {
		Peers []string `json:"peers"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&out); err != nil || len(out.Peers) != 1 {
		t.Fatalf("peers = %+v, %v", out, err)
	}
}

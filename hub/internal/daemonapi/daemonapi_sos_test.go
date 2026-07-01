// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.15 - Phase 14: SOS publish notifies management stream.

package daemonapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
)

type recordingNotifier struct {
	sosNode identity.NodeID
	sosApp  string
	sosBody []byte
}

func (r *recordingNotifier) AppPresenceChanged(identity.NodeID, string, string) {}

func (r *recordingNotifier) SosAlertPublished(nodeID identity.NodeID, appID, topic string, payload []byte) {
	if topic == "sos" {
		r.sosNode = nodeID
		r.sosApp = appID
		r.sosBody = append([]byte(nil), payload...)
	}
}

func TestServer_PublishSosNotifies(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var self identity.NodeID
	self[4] = 0x11
	notifier := &recordingNotifier{}
	srv := New(Config{
		SelfID:     self,
		ListenAddr: "tcp:127.0.0.1:0",
		Apps:       apppresence.NewStore(db),
		Notifier:   notifier,
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	base := "http://" + srv.Addr().String()

	regBody, _ := json.Marshal(map[string]string{
		"app_id": "net.quakemesh.sosbeacon", "app_name": "SOS", "app_version": "0.1.0",
	})
	rec, err := http.Post(base+"/v1/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	var regResp struct {
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&regResp); err != nil {
		t.Fatal(err)
	}
	rec.Body.Close()

	payload := []byte(`{"text":"help","lat":1,"lon":2}`)
	pubBody, _ := json.Marshal(map[string]string{
		"topic":       "sos",
		"payload_b64": base64.StdEncoding.EncodeToString(payload),
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/publish", bytes.NewReader(pubBody))
	req.Header.Set("X-Mesh-Session", regResp.SessionToken)
	rec2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rec2.Body.Close()

	if notifier.sosApp != "net.quakemesh.sosbeacon" {
		t.Fatalf("app = %q", notifier.sosApp)
	}
	if string(notifier.sosBody) != string(payload) {
		t.Fatalf("payload = %q", notifier.sosBody)
	}
}

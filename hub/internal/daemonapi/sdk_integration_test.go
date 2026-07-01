// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.14 - Phase 12: SDK integration smoke test for reference apps.

package daemonapi

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/apppresence"
	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/storage"
	"github.com/JohnDovey/QuakeMesh/sdk"
)

func TestSDK_AllMethods(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	var self identity.NodeID
	self[1] = 0x42
	srv := New(Config{
		SelfID:     self,
		ListenAddr: "tcp:127.0.0.1:0",
		Apps:       apppresence.NewStore(db),
	})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	addr := srv.Addr().String()

	pub := sdk.NewTCPClient(addr)
	sess, err := pub.Register("net.quakemesh.meshdemo", "Mesh Demo", "0.1.0", []string{"demo"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(sess.NodeID) == 0 {
		t.Fatal("session missing node id")
	}

	peers, err := pub.DiscoverPeers("net.quakemesh.privatechat", "")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	_ = peers // no remote peers required for local smoke test

	sub := sdk.NewTCPClient(addr)
	if _, err := sub.Register("net.quakemesh.meshdemo-sub", "Sub", "0.1.0", nil); err != nil {
		t.Fatalf("sub register: %v", err)
	}
	topic := "meshdemo-test"
	ch, err := sub.Subscribe(nil, topic)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := pub.Publish(sess, topic, []byte("ping")); err != nil {
			t.Errorf("publish: %v", err)
		}
	}()

	select {
	case payload := <-ch:
		if string(payload) != "ping" {
			t.Fatalf("payload = %q", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscribe timeout")
	}

	var dst identity.NodeID
	dst[2] = 0x99
	if err := srv.DeliverLocal(dst, self, []byte("inbound")); err != nil {
		t.Fatal(err)
	}
	recvCh, err := pub.Receive(sess)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	select {
	case payload := <-recvCh:
		if string(payload) != "inbound" {
			t.Fatalf("receive = %q", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("receive timeout")
	}
}

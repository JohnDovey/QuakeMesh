// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial tests: frame delivery, simulated loss, simulated
//           latency, peer-up/peer-down notifications, unknown-peer send.

package simnet

import (
	"bytes"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/JohnDovey/QuakeMesh/core/transport"
)

func TestFabric_DeliversFrameBetweenTwoNodes(t *testing.T) {
	fabric := NewFabric(Config{})
	a := fabric.NewNode(transport.PeerID("a"))
	b := fabric.NewNode(transport.PeerID("b"))

	received := make(chan []byte, 1)
	b.OnReceive(func(peer transport.PeerID, frame []byte) {
		if string(peer) != "a" {
			t.Errorf("received frame claims to be from %q, want %q", peer, "a")
		}
		received <- frame
	})

	if err := a.Send(transport.PeerID("b"), []byte("hello")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case frame := <-received:
		if !bytes.Equal(frame, []byte("hello")) {
			t.Fatalf("received %q, want %q", frame, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for frame delivery")
	}
}

func TestFabric_SendToUnknownPeerDoesNotPanic(t *testing.T) {
	fabric := NewFabric(Config{})
	a := fabric.NewNode(transport.PeerID("a"))

	if err := a.Send(transport.PeerID("nonexistent"), []byte("hello")); err != nil {
		t.Fatalf("Send to unknown peer returned error: %v", err)
	}
	// No panic and no delivery -- analogous to a peer being out of range.
}

func TestFabric_FullPacketLossDropsEverything(t *testing.T) {
	fabric := NewFabric(Config{
		PacketLoss: 1.0,
		Rand:       rand.New(rand.NewSource(1)),
	})
	a := fabric.NewNode(transport.PeerID("a"))
	b := fabric.NewNode(transport.PeerID("b"))

	received := make(chan []byte, 1)
	b.OnReceive(func(peer transport.PeerID, frame []byte) { received <- frame })

	for i := 0; i < 20; i++ {
		if err := a.Send(transport.PeerID("b"), []byte("hello")); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	select {
	case frame := <-received:
		t.Fatalf("received frame %q despite 100%% configured packet loss", frame)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing arrives
	}
}

func TestFabric_LatencyDelaysDelivery(t *testing.T) {
	const latency = 100 * time.Millisecond
	fabric := NewFabric(Config{Latency: latency})
	a := fabric.NewNode(transport.PeerID("a"))
	b := fabric.NewNode(transport.PeerID("b"))

	received := make(chan time.Time, 1)
	b.OnReceive(func(peer transport.PeerID, frame []byte) { received <- time.Now() })

	sentAt := time.Now()
	if err := a.Send(transport.PeerID("b"), []byte("hello")); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case arrivedAt := <-received:
		if elapsed := arrivedAt.Sub(sentAt); elapsed < latency {
			t.Fatalf("frame arrived after %v, want at least %v", elapsed, latency)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delayed delivery")
	}
}

func TestFabric_ExistingNodeNotifiedWhenPeerJoins(t *testing.T) {
	fabric := NewFabric(Config{})
	a := fabric.NewNode(transport.PeerID("a"))

	var mu sync.Mutex
	var seenByA []string
	a.OnPeerUp(func(peer transport.PeerID) {
		mu.Lock()
		defer mu.Unlock()
		seenByA = append(seenByA, string(peer))
	})

	fabric.NewNode(transport.PeerID("b"))

	mu.Lock()
	defer mu.Unlock()
	if len(seenByA) != 1 || seenByA[0] != "b" {
		t.Fatalf("a's OnPeerUp saw %v, want [b]", seenByA)
	}
}

func TestFabric_JoiningNodeLearnsAboutExistingPeers(t *testing.T) {
	fabric := NewFabric(Config{})
	fabric.NewNode(transport.PeerID("a"))
	fabric.NewNode(transport.PeerID("b"))

	// c joins after a and b are already present. Registering OnPeerUp
	// must replay the peers c missed at join time.
	c := fabric.NewNode(transport.PeerID("c"))

	var mu sync.Mutex
	seenByC := make(map[string]bool)
	c.OnPeerUp(func(peer transport.PeerID) {
		mu.Lock()
		defer mu.Unlock()
		seenByC[string(peer)] = true
	})

	mu.Lock()
	defer mu.Unlock()
	if !seenByC["a"] || !seenByC["b"] || len(seenByC) != 2 {
		t.Fatalf("c's OnPeerUp replay saw %v, want exactly {a, b}", seenByC)
	}
}

func TestFabric_NotifiesPeerDownOnRemove(t *testing.T) {
	fabric := NewFabric(Config{})
	a := fabric.NewNode(transport.PeerID("a"))
	fabric.NewNode(transport.PeerID("b"))

	down := make(chan string, 1)
	a.OnPeerDown(func(peer transport.PeerID) { down <- string(peer) })

	fabric.RemoveNode(transport.PeerID("b"))

	select {
	case peer := <-down:
		if peer != "b" {
			t.Fatalf("OnPeerDown fired for %q, want %q", peer, "b")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OnPeerDown")
	}
}

// TestFabric_HandlerCanCallBackIntoFabric guards against a deadlock
// regression: a real routing/trust implementation will want to react to
// OnPeerUp by immediately Send-ing a "hello" (see "Node/Hub Presence and
// Discovery" in /plan.md), so callbacks must never fire while the
// fabric's internal lock is held.
func TestFabric_HandlerCanCallBackIntoFabric(t *testing.T) {
	fabric := NewFabric(Config{})
	a := fabric.NewNode(transport.PeerID("a"))
	b := fabric.NewNode(transport.PeerID("b"))

	helloReceived := make(chan struct{}, 1)
	b.OnReceive(func(peer transport.PeerID, frame []byte) { helloReceived <- struct{}{} })

	done := make(chan struct{})
	a.OnPeerUp(func(peer transport.PeerID) {
		if err := a.Send(peer, []byte("hello")); err != nil {
			t.Errorf("Send from within OnPeerUp handler: %v", err)
		}
		close(done)
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out: OnPeerUp handler's Send call appears to have deadlocked")
	}

	select {
	case <-helloReceived:
	case <-time.After(time.Second):
		t.Fatal("hello sent from within OnPeerUp handler was never delivered")
	}
}

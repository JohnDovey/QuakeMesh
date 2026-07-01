// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.3 - Initial tests: WebSocket event delivery, multi-client
//           fan-out, and the ogmengine.EventHandler adapter methods.

package managementapi

import (
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/wire"
	"github.com/JohnDovey/QuakeMesh/hub/internal/registry"
)

func startTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s := New("127.0.0.1:0")
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	wsURL := (&url.URL{Scheme: "ws", Host: s.Addr().String(), Path: "/ws"}).String()
	return s, wsURL
}

func dialTestClient(t *testing.T, wsURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", wsURL, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestServer_DeliversPublishedEvent(t *testing.T) {
	s, wsURL := startTestServer(t)
	conn := dialTestClient(t, wsURL)

	// Give the server a moment to register the client before publishing;
	// Publish drops events for clients that have not yet connected.
	time.Sleep(50 * time.Millisecond)

	want := &wire.ManagementEvent{
		EmittedAtUnixMs: 123,
		Event: &wire.ManagementEvent_DtnQueueDepthChanged{
			DtnQueueDepthChanged: &wire.DtnQueueDepthChanged{Depth: 7},
		},
	}
	s.Publish(want)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("message type = %d, want BinaryMessage", msgType)
	}

	var got wire.ManagementEvent
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}
	if got.EmittedAtUnixMs != want.EmittedAtUnixMs {
		t.Fatalf("EmittedAtUnixMs = %d, want %d", got.EmittedAtUnixMs, want.EmittedAtUnixMs)
	}
	gotDepth := got.GetDtnQueueDepthChanged()
	if gotDepth == nil || gotDepth.Depth != 7 {
		t.Fatalf("DtnQueueDepthChanged = %+v, want Depth=7", gotDepth)
	}
}

func TestServer_FansOutToMultipleClients(t *testing.T) {
	s, wsURL := startTestServer(t)
	connA := dialTestClient(t, wsURL)
	connB := dialTestClient(t, wsURL)

	time.Sleep(50 * time.Millisecond)

	s.Publish(&wire.ManagementEvent{EmittedAtUnixMs: 1})

	for name, conn := range map[string]*websocket.Conn{"A": connA, "B": connB} {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("client %s did not receive the published event: %v", name, err)
		}
	}
}

func TestServer_RemovesClientOnDisconnect(t *testing.T) {
	s, wsURL := startTestServer(t)
	conn := dialTestClient(t, wsURL)
	time.Sleep(50 * time.Millisecond)

	s.mu.Lock()
	before := len(s.clients)
	s.mu.Unlock()
	if before != 1 {
		t.Fatalf("connected client count = %d, want 1", before)
	}

	conn.Close()
	time.Sleep(100 * time.Millisecond)

	// Trigger the write path so the server notices the broken connection
	// and unregisters it (see handleWS's done-channel/select loop).
	s.Publish(&wire.ManagementEvent{EmittedAtUnixMs: 1})
	time.Sleep(100 * time.Millisecond)

	s.mu.Lock()
	after := len(s.clients)
	s.mu.Unlock()
	if after != 0 {
		t.Fatalf("connected client count after disconnect = %d, want 0", after)
	}
}

func TestServer_NodeStatusChangedAdapter(t *testing.T) {
	s, wsURL := startTestServer(t)
	conn := dialTestClient(t, wsURL)
	time.Sleep(50 * time.Millisecond)

	var id identity.NodeID
	id[0] = 0xAB
	s.NodeStatusChanged(id, registry.NodeStatusStale)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var got wire.ManagementEvent
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}
	changed := got.GetNodeStatusChanged()
	if changed == nil || changed.Status != "stale" || changed.NodeId[0] != 0xAB {
		t.Fatalf("NodeStatusChanged = %+v, want status=stale node_id[0]=0xAB", changed)
	}
}

func TestServer_RouteChangedAdapter(t *testing.T) {
	s, wsURL := startTestServer(t)
	conn := dialTestClient(t, wsURL)
	time.Sleep(50 * time.Millisecond)

	var dst, nextHop identity.NodeID
	dst[0], nextHop[0] = 0x01, 0x02
	s.RouteChanged(registry.Route{Destination: dst, NextHop: nextHop, TQ: 0.75, HopCount: 2})

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var got wire.ManagementEvent
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}
	route := got.GetRouteChanged()
	if route == nil || route.Tq != 0.75 || route.HopCount != 2 || route.DstNodeId[0] != 0x01 || route.NextHopNodeId[0] != 0x02 {
		t.Fatalf("RouteChanged = %+v, want tq=0.75 hop_count=2 dst[0]=1 next_hop[0]=2", route)
	}
}

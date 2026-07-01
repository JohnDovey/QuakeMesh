// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: WebSocket client subscribing to Hub management
//           events and fanning JSON to browser subscribers.

package hubclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/JohnDovey/QuakeMesh/core/identity"
	"github.com/JohnDovey/QuakeMesh/core/wire"
)

// Event is a JSON-serialisable dashboard event for browser /ws clients.
type Event struct {
	Type string `json:"type"`

	EmittedAtUnixMs int64   `json:"emitted_at_unix_ms,omitempty"`
	NodeID          string  `json:"node_id,omitempty"`
	HubID           string  `json:"hub_id,omitempty"`
	Status          string  `json:"status,omitempty"`
	Destination     string  `json:"destination,omitempty"`
	NextHop         string  `json:"next_hop,omitempty"`
	TQ              float64 `json:"tq,omitempty"`
	HopCount        uint32  `json:"hop_count,omitempty"`
	DTNDepth        uint32  `json:"dtn_depth,omitempty"`
	Enabled         bool    `json:"enabled,omitempty"`
	AppID           string  `json:"app_id,omitempty"`
	AppVersion      string  `json:"app_version,omitempty"`
}

// Client subscribes to QuakeMeshHub's loopback /ws management stream.
type Client struct {
	hubWSURL string

	mu       sync.RWMutex
	listeners map[chan Event]struct{}
}

// New creates a hub event client for hubWSURL (e.g. ws://127.0.0.1:8083/ws).
func New(hubWSURL string) *Client {
	return &Client{
		hubWSURL:  hubWSURL,
		listeners: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a listener channel. The caller must not close ch;
// Unsubscribe removes it.
func (c *Client) Subscribe(ch chan Event) {
	c.mu.Lock()
	c.listeners[ch] = struct{}{}
	c.mu.Unlock()
}

// Unsubscribe removes a listener channel.
func (c *Client) Unsubscribe(ch chan Event) {
	c.mu.Lock()
	delete(c.listeners, ch)
	c.mu.Unlock()
}

// Run connects to the Hub and reconnects on failure until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.runOnce(ctx); err != nil && ctx.Err() == nil {
			log.Printf("hubclient: %v; reconnecting in %s", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	u, err := url.Parse(c.hubWSURL)
	if err != nil {
		return fmt.Errorf("parse hub url: %w", err)
	}
	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial hub: %w", err)
	}
	defer conn.Close()
	log.Printf("hubclient: connected to %s", c.hubWSURL)

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read hub event: %w", err)
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		var mgmt wire.ManagementEvent
		if err := proto.Unmarshal(data, &mgmt); err != nil {
			log.Printf("hubclient: unmarshal: %v", err)
			continue
		}
		if ev, ok := translateEvent(&mgmt); ok {
			c.broadcast(ev)
		}
		backoff = time.Second
		_ = backoff
	}
}

func (c *Client) broadcast(ev Event) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for ch := range c.listeners {
		select {
		case ch <- ev:
		default:
		}
	}
}

func translateEvent(mgmt *wire.ManagementEvent) (Event, bool) {
	base := Event{
		Type:            "unknown",
		EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
	}
	switch e := mgmt.Event.(type) {
	case *wire.ManagementEvent_NodeStatusChanged:
		return Event{
			Type:            "node_status_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			NodeID:          nodeIDString(e.NodeStatusChanged.NodeId),
			Status:          e.NodeStatusChanged.Status,
		}, true
	case *wire.ManagementEvent_HubStatusChanged:
		return Event{
			Type:            "hub_status_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			HubID:           nodeIDString(e.HubStatusChanged.HubId),
			Status:          e.HubStatusChanged.Status,
		}, true
	case *wire.ManagementEvent_RouteChanged:
		return Event{
			Type:            "route_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			Destination:     nodeIDString(e.RouteChanged.DstNodeId),
			NextHop:         nodeIDString(e.RouteChanged.NextHopNodeId),
			TQ:              e.RouteChanged.Tq,
			HopCount:        e.RouteChanged.HopCount,
		}, true
	case *wire.ManagementEvent_DtnQueueDepthChanged:
		return Event{
			Type:            "dtn_queue_depth_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			DTNDepth:        e.DtnQueueDepthChanged.Depth,
		}, true
	case *wire.ManagementEvent_InternetFallbackChanged:
		return Event{
			Type:            "internet_fallback_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			Enabled:         e.InternetFallbackChanged.Enabled,
		}, true
	case *wire.ManagementEvent_AppPresenceChanged:
		return Event{
			Type:            "app_presence_changed",
			EmittedAtUnixMs: mgmt.EmittedAtUnixMs,
			NodeID:          nodeIDString(e.AppPresenceChanged.NodeId),
			AppID:           e.AppPresenceChanged.AppId,
			AppVersion:      e.AppPresenceChanged.AppVersion,
		}, true
	default:
		return base, false
	}
}

func nodeIDString(b []byte) string {
	if len(b) != len(identity.NodeID{}) {
		return fmt.Sprintf("%x", b)
	}
	var id identity.NodeID
	copy(id[:], b)
	return id.String()
}

// MarshalJSON is re-exported for tests.
func (e Event) MarshalJSONBytes() ([]byte, error) {
	return json.Marshal(e)
}

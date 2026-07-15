// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.18 - LAN multicast presence beacons for hub/node discovery.
//   0.0.19 - optional lan_context on hub/node beacons.

// Package lanbeacon defines the JSON UDP multicast format used on the
// connected Wi-Fi LAN (see "LAN discovery" in /plan.md).
package lanbeacon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JohnDovey/QuakeMesh/core/lancontext"
)

const (
	// MulticastGroup is the LAN discovery multicast address.
	MulticastGroup = "239.255.42.99"
	// MulticastPort is shared by hub and node presence beacons.
	MulticastPort = 47223
)

const (
	magic0 = 'Q'
	magic1 = 'M'
	magic2 = 'L'
	magic3 = 'B'
	version = 1
)

const (
	KindHub  = "hub"
	KindNode = "node"
)

// Message is a presence beacon payload.
type Message struct {
	V             int                 `json:"v"`
	Kind          string              `json:"kind"`
	NodeID        string              `json:"node_id"`
	Handle        string              `json:"handle,omitempty"`
	HomeLat       *float64            `json:"home_lat,omitempty"`
	HomeLon       *float64            `json:"home_lon,omitempty"`
	HeartbeatPort int                 `json:"heartbeat_port,omitempty"`
	OGMPort       int                 `json:"ogm_port,omitempty"`
	Lat           *float64            `json:"lat,omitempty"`
	Lon           *float64            `json:"lon,omitempty"`
	AccuracyM     *float64            `json:"accuracy_m,omitempty"`
	LanContext    *lancontext.Context `json:"lan_context,omitempty"`
}

// Encode marshals a beacon with the QuakeMesh LAN prefix.
func Encode(msg Message) ([]byte, error) {
	if msg.V == 0 {
		msg.V = 1
	}
	if msg.Kind == "" {
		return nil, errors.New("lanbeacon: kind required")
	}
	if msg.NodeID == "" {
		return nil, errors.New("lanbeacon: node_id required")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 5+len(body))
	out = append(out, magic0, magic1, magic2, magic3, version)
	out = append(out, body...)
	return out, nil
}

// Decode parses a beacon datagram. Returns false when the payload is not a
// QuakeMesh LAN beacon (e.g. a mesh frame on the same port).
func Decode(payload []byte) (Message, bool, error) {
	if len(payload) < 6 || payload[0] != magic0 || payload[1] != magic1 ||
		payload[2] != magic2 || payload[3] != magic3 || payload[4] != version {
		return Message{}, false, nil
	}
	var msg Message
	if err := json.Unmarshal(payload[5:], &msg); err != nil {
		return Message{}, true, err
	}
	if msg.Kind == "" || msg.NodeID == "" {
		return Message{}, true, errors.New("lanbeacon: invalid message")
	}
	return msg, true, nil
}

// IsBeacon reports whether payload starts with the QuakeMesh LAN prefix.
func IsBeacon(payload []byte) bool {
	_, ok, _ := Decode(payload)
	return ok
}

// HubBeacon builds a hub announcement.
func HubBeacon(nodeID string, heartbeatPort, ogmPort int, lan *lancontext.Context) ([]byte, error) {
	if heartbeatPort <= 0 || ogmPort <= 0 {
		return nil, fmt.Errorf("lanbeacon: invalid hub ports heartbeat=%d ogm=%d", heartbeatPort, ogmPort)
	}
	return Encode(Message{
		V:             1,
		Kind:          KindHub,
		NodeID:        nodeID,
		HeartbeatPort: heartbeatPort,
		OGMPort:       ogmPort,
		LanContext:    lan,
	})
}

// NodeBeacon builds a node announcement.
func NodeBeacon(nodeID string, handle string, homeLat, homeLon, lat, lon, accuracyM *float64, lan *lancontext.Context) ([]byte, error) {
	return Encode(Message{
		V:          1,
		Kind:       KindNode,
		NodeID:     nodeID,
		Handle:     handle,
		HomeLat:    homeLat,
		HomeLon:    homeLon,
		Lat:        lat,
		Lon:        lon,
		AccuracyM:  accuracyM,
		LanContext: lan,
	})
}

// Prefix returns the wire prefix bytes (for tests).
func Prefix() []byte {
	return bytes.Clone([]byte{magic0, magic1, magic2, magic3, version})
}

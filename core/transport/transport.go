// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package transport defines the abstract link interface mesh-core depends
// on. Platform-specific implementations (BLE, Wi-Fi Direct, Wi-Fi Aware,
// LAN, internet fallback) are plugged in by the host binary or the Android
// Kotlin layer; mesh-core itself never references a concrete transport.
package transport

// PeerID identifies the far end of a link, scoped to a single Transport.
type PeerID []byte

// Transport is the interface every link implementation must satisfy.
type Transport interface {
	Send(peer PeerID, frame []byte) error
	OnReceive(handler func(peer PeerID, frame []byte))
	OnPeerUp(handler func(peer PeerID))
	OnPeerDown(handler func(peer PeerID))
}

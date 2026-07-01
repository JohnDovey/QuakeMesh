// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.12 - Phase 10: HTTPClient implements Client against local daemon.

// Package sdk wraps the local IPC API (Unix domain socket on Hub, bound
// service / loopback gRPC on Android) so third-party CLI/server apps can
// use the mesh purely as a transport. See "Application SDK and
// Transport-as-a-Service" in /plan.md.
package sdk

// Session is returned by Register and threaded through subsequent calls.
type Session struct {
	AppID      string
	AppName    string
	AppVersion string
	NodeID     []byte // local mesh node id (hex-decoded from register response)
}

// Client is the mesh-sdk surface every app integrates against.
type Client interface {
	Register(appID, appName, appVersion string, capabilities []string) (*Session, error)
	Send(session *Session, destNodeID []byte, payload []byte) error
	Receive(session *Session) (<-chan []byte, error)
	Publish(session *Session, topic string, payload []byte) error
	Subscribe(session *Session, topic string) (<-chan []byte, error)
	DiscoverPeers(appID string, versionConstraint string) ([][]byte, error)
}

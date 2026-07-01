// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.7 - Phase 6: SQLite-backed bundle queue (Store).

// Package dtn implements the store-and-forward bundle queue (dtq_queue):
// store-carry-forward per RFC 9171, with TTL/expiry and retry count so
// bundles do not accumulate indefinitely. See "Store-and-Forward (DTN
// Queue)" in /plan.md.
package dtn

// Bundle is a queued, undeliverable payload awaiting a route or
// internet-fallback permission.
type Bundle struct {
	BundleID      [16]byte
	SrcNodeID     [32]byte
	DstNodeID     [32]byte
	Payload       []byte
	ExpiresAtUnix int64
	RetryCount    uint32
}

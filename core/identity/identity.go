// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Package identity will implement self-sovereign node identity: an Ed25519
// keypair generated on first run, with NodeID derived as the SHA-256 hash
// of the public key. There is no central CA and no registration step.
//
// See "Identity and Security" in /plan.md. Not yet implemented (Phase 1).
package identity

// NodeID uniquely identifies a node or hub on the mesh.
type NodeID [32]byte

// Identity holds a node's keypair and derived NodeID.
type Identity struct {
	NodeID NodeID
}

// New will generate a fresh Ed25519 keypair and derive its NodeID.
func New() (*Identity, error) {
	panic("identity.New: not implemented")
}

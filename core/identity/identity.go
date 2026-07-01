// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.2 - Implemented Ed25519 keygen, NodeID derivation, disk
//           persistence (LoadOrCreate), and Sign/Verify.

// Package identity implements self-sovereign node identity: an Ed25519
// keypair generated on first run, with NodeID derived as the SHA-256 hash
// of the public key. There is no central CA and no registration step.
//
// See "Identity and Security" in /plan.md.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// NodeID uniquely identifies a node or hub on the mesh.
type NodeID [32]byte

// String returns the hex encoding of the NodeID.
func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

// Identity holds a node's keypair and derived NodeID.
type Identity struct {
	NodeID     NodeID
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// New generates a fresh Ed25519 keypair and derives its NodeID.
func New() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: generate key: %w", err)
	}
	return fromKeys(pub, priv), nil
}

// LoadOrCreate reads the Ed25519 seed stored at path, or generates a new
// keypair and persists its seed to path (mode 0600) if the file does not
// yet exist. path's parent directory must already exist.
func LoadOrCreate(path string) (*Identity, error) {
	seed, err := os.ReadFile(path)
	switch {
	case err == nil:
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("identity: %s: expected %d-byte seed, got %d", path, ed25519.SeedSize, len(seed))
		}
		priv := ed25519.NewKeyFromSeed(seed)
		return fromKeys(priv.Public().(ed25519.PublicKey), priv), nil
	case os.IsNotExist(err):
		id, genErr := New()
		if genErr != nil {
			return nil, genErr
		}
		if writeErr := os.WriteFile(path, id.PrivateKey.Seed(), 0o600); writeErr != nil {
			return nil, fmt.Errorf("identity: persist seed to %s: %w", path, writeErr)
		}
		return id, nil
	default:
		return nil, fmt.Errorf("identity: read %s: %w", path, err)
	}
}

// Sign signs message with this identity's private key.
func (id *Identity) Sign(message []byte) []byte {
	return ed25519.Sign(id.PrivateKey, message)
}

// Verify reports whether sig is a valid signature of message by pub.
func Verify(pub ed25519.PublicKey, message, sig []byte) bool {
	return ed25519.Verify(pub, message, sig)
}

func fromKeys(pub ed25519.PublicKey, priv ed25519.PrivateKey) *Identity {
	return &Identity{
		NodeID:     sha256.Sum256(pub),
		PublicKey:  pub,
		PrivateKey: priv,
	}
}

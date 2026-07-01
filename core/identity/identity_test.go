// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial tests for keygen, NodeID derivation, persistence,
//           and Sign/Verify.

package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_DistinctIdentities(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.NodeID == b.NodeID {
		t.Fatal("two calls to New produced the same NodeID")
	}
}

func TestNew_NodeIDIsSHA256OfPublicKey(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := sha256.Sum256(id.PublicKey)
	if id.NodeID != NodeID(want) {
		t.Fatalf("NodeID = %x, want sha256(pubkey) = %x", id.NodeID, want)
	}
}

func TestLoadOrCreate_PersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.seed")

	first, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate (create): %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat seed file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("seed file mode = %o, want 0600", perm)
	}

	second, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate (reload): %v", err)
	}

	if first.NodeID != second.NodeID {
		t.Fatalf("NodeID changed across reload: %x != %x", first.NodeID, second.NodeID)
	}
	if !bytes.Equal(first.PublicKey, second.PublicKey) {
		t.Fatal("public key changed across reload")
	}
}

func TestLoadOrCreate_RejectsCorruptSeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.seed")
	if err := os.WriteFile(path, []byte("too short"), 0o600); err != nil {
		t.Fatalf("write corrupt seed: %v", err)
	}
	if _, err := LoadOrCreate(path); err == nil {
		t.Fatal("LoadOrCreate accepted a corrupt seed file")
	}
}

func TestSignVerify(t *testing.T) {
	id, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	message := []byte("quakemesh")
	sig := id.Sign(message)

	if !Verify(id.PublicKey, message, sig) {
		t.Fatal("Verify rejected a valid signature")
	}
	if Verify(id.PublicKey, []byte("tampered"), sig) {
		t.Fatal("Verify accepted a signature over the wrong message")
	}

	other, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if Verify(other.PublicKey, message, sig) {
		t.Fatal("Verify accepted a signature under the wrong public key")
	}
}

func TestVerify_UsesStandardEd25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	message := []byte("interop")
	sig := ed25519.Sign(priv, message)
	if !Verify(pub, message, sig) {
		t.Fatal("Verify rejected a signature produced directly by crypto/ed25519")
	}
}

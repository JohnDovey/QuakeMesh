// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.5 - Phase 4: gomobile bind tests for the mobile Node facade.

package mobile

import (
	"path/filepath"
	"testing"
)

type recordingSink struct {
	peer  string
	frame []byte
}

func (r *recordingSink) SendFrame(peerHex string, frame []byte) {
	r.peer = peerHex
	r.frame = append([]byte(nil), frame...)
}

func TestNewNode_andEmitFrame(t *testing.T) {
	dir := t.TempDir()
	n, err := NewNode(
		filepath.Join(dir, "node.identity"),
		filepath.Join(dir, "quakemesh.db"),
	)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	defer n.Close()

	if n.NodeID() == "" {
		t.Fatal("expected non-empty NodeID")
	}

	sink := &recordingSink{}
	n.SetFrameSink(sink)
	payload := []byte("hello-mesh")
	if err := n.EmitFrame("abc", payload); err != nil {
		t.Fatalf("EmitFrame: %v", err)
	}
	if string(sink.frame) != "hello-mesh" {
		t.Fatalf("frame = %q", sink.frame)
	}
}

func TestOnFrameReceived_rejectsInvalidPeerHex(t *testing.T) {
	dir := t.TempDir()
	n, err := NewNode(
		filepath.Join(dir, "node.identity"),
		filepath.Join(dir, "quakemesh.db"),
	)
	if err != nil {
		t.Fatalf("NewNode: %v", err)
	}
	defer n.Close()

	if err := n.OnFrameReceived("not-hex!", []byte("x")); err == nil {
		t.Fatal("expected error for invalid peer hex")
	}
}

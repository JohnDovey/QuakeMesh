// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial tests: full Noise_XX handshake, bidirectional
//           transport encryption, and tamper rejection.

package noise

import (
	"bytes"
	"testing"
)

// handshake drives a full Noise_XX exchange (-> e; <- e,ee,s,es; -> s,se)
// between an initiator and a responder and returns both established
// sessions.
func handshake(t *testing.T) (initiator, responder *Session) {
	t.Helper()

	initKeys, err := GenerateStaticKeypair()
	if err != nil {
		t.Fatalf("GenerateStaticKeypair (initiator): %v", err)
	}
	respKeys, err := GenerateStaticKeypair()
	if err != nil {
		t.Fatalf("GenerateStaticKeypair (responder): %v", err)
	}

	initiator, err = NewInitiator(initKeys)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}
	responder, err = NewResponder(respKeys)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}

	msg1, err := initiator.WriteHandshakeMessage(nil)
	if err != nil {
		t.Fatalf("initiator write msg1: %v", err)
	}
	if _, err := responder.ReadHandshakeMessage(msg1); err != nil {
		t.Fatalf("responder read msg1: %v", err)
	}

	msg2, err := responder.WriteHandshakeMessage(nil)
	if err != nil {
		t.Fatalf("responder write msg2: %v", err)
	}
	if _, err := initiator.ReadHandshakeMessage(msg2); err != nil {
		t.Fatalf("initiator read msg2: %v", err)
	}

	msg3, err := initiator.WriteHandshakeMessage(nil)
	if err != nil {
		t.Fatalf("initiator write msg3: %v", err)
	}
	if _, err := responder.ReadHandshakeMessage(msg3); err != nil {
		t.Fatalf("responder read msg3: %v", err)
	}

	if !initiator.Established() {
		t.Fatal("initiator not established after 3-message XX handshake")
	}
	if !responder.Established() {
		t.Fatal("responder not established after 3-message XX handshake")
	}

	if !bytes.Equal(initiator.PeerStatic(), respKeys.Public) {
		t.Fatal("initiator's view of peer static key does not match responder's actual static key")
	}
	if !bytes.Equal(responder.PeerStatic(), initKeys.Public) {
		t.Fatal("responder's view of peer static key does not match initiator's actual static key")
	}

	return initiator, responder
}

func TestHandshake_Establishes(t *testing.T) {
	handshake(t)
}

func TestSession_BidirectionalTransport(t *testing.T) {
	initiator, responder := handshake(t)

	fromInitiator := []byte("hello from initiator")
	ct, err := initiator.Encrypt(fromInitiator)
	if err != nil {
		t.Fatalf("initiator Encrypt: %v", err)
	}
	pt, err := responder.Decrypt(ct)
	if err != nil {
		t.Fatalf("responder Decrypt: %v", err)
	}
	if !bytes.Equal(pt, fromInitiator) {
		t.Fatalf("responder decrypted %q, want %q", pt, fromInitiator)
	}

	fromResponder := []byte("hello from responder")
	ct2, err := responder.Encrypt(fromResponder)
	if err != nil {
		t.Fatalf("responder Encrypt: %v", err)
	}
	pt2, err := initiator.Decrypt(ct2)
	if err != nil {
		t.Fatalf("initiator Decrypt: %v", err)
	}
	if !bytes.Equal(pt2, fromResponder) {
		t.Fatalf("initiator decrypted %q, want %q", pt2, fromResponder)
	}
}

func TestSession_RejectsTamperedCiphertext(t *testing.T) {
	initiator, responder := handshake(t)

	ct, err := initiator.Encrypt([]byte("integrity check"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := bytes.Clone(ct)
	tampered[0] ^= 0xFF

	if _, err := responder.Decrypt(tampered); err == nil {
		t.Fatal("Decrypt accepted a tampered ciphertext")
	}
}

func TestEncryptDecrypt_BeforeHandshake(t *testing.T) {
	keys, err := GenerateStaticKeypair()
	if err != nil {
		t.Fatalf("GenerateStaticKeypair: %v", err)
	}
	s, err := NewInitiator(keys)
	if err != nil {
		t.Fatalf("NewInitiator: %v", err)
	}

	if _, err := s.Encrypt([]byte("too early")); err != ErrNotEstablished {
		t.Fatalf("Encrypt before handshake: err = %v, want ErrNotEstablished", err)
	}
	if _, err := s.Decrypt([]byte("too early")); err != ErrNotEstablished {
		t.Fatalf("Decrypt before handshake: err = %v, want ErrNotEstablished", err)
	}
}

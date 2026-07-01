// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.2 - Initial Noise_XX session wrapper (per-hop link encryption).

// Package noise implements the per-hop link encryption every radio
// transport (Bluetooth, Wi-Fi Direct, LAN UDP) is wrapped in: a Noise
// Protocol handshake, the same primitive used by WireGuard, so adjacent
// peers authenticate each other and encrypt every frame. See "Identity
// and Security" in /plan.md.
//
// Noise_XX is used: neither side needs prior knowledge of the other's
// static public key, matching mesh peers that discover each other ad
// hoc. Once Session.Established reports true, the caller should compare
// PeerStatic() against whatever the peer's advertised static key is
// (e.g. from presence/gossip data) before trusting the link.
package noise

import (
	"errors"

	"github.com/flynn/noise"
)

var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashBLAKE2s)

// StaticKeypair is a Noise static Curve25519 keypair, distinct from a
// node's Ed25519 identity keypair (see /core/identity). It authenticates
// a link endpoint for the lifetime of the handshake, not the node's
// long-term mesh identity.
type StaticKeypair = noise.DHKey

// GenerateStaticKeypair generates a fresh Noise static keypair.
func GenerateStaticKeypair() (StaticKeypair, error) {
	return cipherSuite.GenerateKeypair(nil)
}

// ErrNotEstablished is returned by Encrypt/Decrypt before the handshake
// has completed.
var ErrNotEstablished = errors.New("noise: handshake not established")

// Session drives one Noise_XX handshake and the resulting transport
// encryption for a single link to a single peer.
type Session struct {
	hs          *noise.HandshakeState
	initiator   bool
	established bool
	sendCipher  *noise.CipherState
	recvCipher  *noise.CipherState
}

func newSession(initiator bool, local StaticKeypair) (*Session, error) {
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   cipherSuite,
		Pattern:       noise.HandshakeXX,
		Initiator:     initiator,
		StaticKeypair: local,
	})
	if err != nil {
		return nil, err
	}
	return &Session{hs: hs, initiator: initiator}, nil
}

// NewInitiator starts a Noise_XX handshake as the side that sends the
// first message.
func NewInitiator(local StaticKeypair) (*Session, error) {
	return newSession(true, local)
}

// NewResponder starts a Noise_XX handshake as the side that receives the
// first message.
func NewResponder(local StaticKeypair) (*Session, error) {
	return newSession(false, local)
}

// WriteHandshakeMessage produces the next handshake message this side
// must send, optionally carrying an application payload piggybacked on
// the handshake. It is an error to call this out of turn.
func (s *Session) WriteHandshakeMessage(payload []byte) ([]byte, error) {
	out, cs0, cs1, err := s.hs.WriteMessage(nil, payload)
	if err != nil {
		return nil, err
	}
	s.maybeFinish(cs0, cs1)
	return out, nil
}

// ReadHandshakeMessage consumes the next handshake message received
// from the peer, returning any application payload piggybacked on it.
func (s *Session) ReadHandshakeMessage(msg []byte) ([]byte, error) {
	payload, cs0, cs1, err := s.hs.ReadMessage(nil, msg)
	if err != nil {
		return nil, err
	}
	s.maybeFinish(cs0, cs1)
	return payload, nil
}

// maybeFinish records the split transport ciphers once the Noise library
// returns them, which happens on whichever WriteMessage/ReadMessage call
// completes the final handshake step. Per the Noise spec, cs0 is bound to
// messages sent by the handshake's initiator and cs1 to messages sent by
// the responder.
func (s *Session) maybeFinish(cs0, cs1 *noise.CipherState) {
	if cs0 == nil {
		return
	}
	if s.initiator {
		s.sendCipher, s.recvCipher = cs0, cs1
	} else {
		s.sendCipher, s.recvCipher = cs1, cs0
	}
	s.established = true
}

// Established reports whether the handshake has completed and Encrypt/
// Decrypt are ready to use.
func (s *Session) Established() bool {
	return s.established
}

// PeerStatic returns the peer's static public key, available once the
// remote side's static key has been transmitted (partway through the
// Noise_XX handshake, before it necessarily completes).
func (s *Session) PeerStatic() []byte {
	return s.hs.PeerStatic()
}

// Encrypt seals plaintext for the peer. Only valid once Established.
func (s *Session) Encrypt(plaintext []byte) ([]byte, error) {
	if !s.established {
		return nil, ErrNotEstablished
	}
	return s.sendCipher.Encrypt(nil, nil, plaintext)
}

// Decrypt opens a ciphertext received from the peer. Only valid once
// Established.
func (s *Session) Decrypt(ciphertext []byte) ([]byte, error) {
	if !s.established {
		return nil, ErrNotEstablished
	}
	return s.recvCipher.Decrypt(nil, nil, ciphertext)
}

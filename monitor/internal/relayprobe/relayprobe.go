// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.4 - Phase 3: TCP reachability probe for relay hub entries.

package relayprobe

import (
	"fmt"
	"net"
	"time"
)

// Probe attempts a TCP connection to ip:port. Phase 3 treats a successful
// TCP handshake as verified reachability; a full relay-capability handshake
// is added when the Hub relay server lands in Phase 9.
func Probe(ip string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("relay probe %s: %w", addr, err)
	}
	return conn.Close()
}

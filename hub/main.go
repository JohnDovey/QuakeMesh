// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Command quakemeshhub is the stable-backbone binary: registry, routing,
// NAT relay, and Hub-to-Hub sync. See "Project Names" in /plan.md.
//
// Not yet implemented (Phase 2). This is a scaffold placeholder.
package main

import (
	"fmt"

	"github.com/JohnDovey/QuakeMesh/core/transport"
)

// loopbackManagementAddr is the Hub's local-only management API address,
// consumed by QuakeMeshMonitor (see /monitor).
const loopbackManagementAddr = "127.0.0.1:8083"

var _ transport.Transport // referenced to confirm /core is wired up

func main() {
	fmt.Println("quakemeshhub", Version, "- scaffold only, not yet implemented (Phase 2)")
	fmt.Println("loopback management API will listen on", loopbackManagementAddr)
}

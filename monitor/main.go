// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.

// Command quakemeshmonitor is the web-based admin and monitoring
// dashboard that runs alongside a QuakeMeshHub. All static assets are
// embedded via go:embed so the binary is self-contained and works fully
// offline. See "QuakeMeshMonitor" in /plan.md.
//
// Not yet implemented (Phase 3). This is a scaffold placeholder.
package main

import (
	"embed"
	"fmt"
	"os"
)

//go:embed static
var staticAssets embed.FS

const defaultPort = "8082"

func main() {
	port := os.Getenv("QUAKEMESH_MONITOR_PORT")
	if port == "" {
		port = defaultPort
	}
	_ = staticAssets // referenced to confirm go:embed wiring compiles
	fmt.Println("quakemeshmonitor", Version, "- scaffold only, not yet implemented (Phase 3)")
	fmt.Println("will bind 0.0.0.0:" + port)
}

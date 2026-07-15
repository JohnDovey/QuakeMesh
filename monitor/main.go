// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.4 - Phase 3: CLI flags wired into monitorapp.Monitor; HTTP
//           dashboard on :8082 with admin auth and Hub event stream.
//   1.0.2 - MeshSniff GET /sniff (+ /api/sniff) on the dashboard HTTP.

// Command quakemeshmonitor is the web-based admin and monitoring
// dashboard that runs alongside a QuakeMeshHub. All static assets are
// embedded via go:embed so the binary is self-contained and works fully
// offline. See "QuakeMeshMonitor" in /plan.md.
package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/JohnDovey/QuakeMesh/monitor/internal/hubdb"
	"github.com/JohnDovey/QuakeMesh/monitor/internal/monitorapp"
)

//go:embed static
var staticAssets embed.FS

const defaultPort = "8082"

func main() {
	bindAddr := flag.String("bind", "0.0.0.0:"+defaultPort, "HTTP bind address")
	hubDB := flag.String("hub-db", "quakemeshhub.db", "path to QuakeMeshHub's SQLite registry (shared)")
	hubWS := flag.String("hub-ws", "ws://127.0.0.1:8083/ws", "QuakeMeshHub loopback management WebSocket URL")
	flag.Parse()

	if port := os.Getenv("QUAKEMESH_MONITOR_PORT"); port != "" && *bindAddr == "0.0.0.0:"+defaultPort {
		*bindAddr = "0.0.0.0:" + port
	}

	dbPath, dbHint := hubdb.Resolve(*hubDB)
	if dbHint != "" {
		log.Print(dbHint)
	}

	fmt.Printf("quakemeshmonitor %s\n", Version)

	mon, err := monitorapp.New(monitorapp.Config{
		BindAddr:   *bindAddr,
		HubDBPath:  dbPath,
		HubWSURL:   *hubWS,
		AppVersion: Version,
	}, staticAssets)
	if err != nil {
		log.Fatalf("quakemeshmonitor: %v", err)
	}

	if err := mon.Start(); err != nil {
		log.Fatalf("quakemeshmonitor: %v", err)
	}
	fmt.Printf("dashboard listening on http://%s\n", mon.Addr())
	fmt.Printf("reading hub registry from %s\n", dbPath)
	fmt.Printf("subscribing to hub events at %s\n", *hubWS)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("shutting down...")
	if err := mon.Close(); err != nil {
		log.Printf("quakemeshmonitor: shutdown: %v", err)
	}
}

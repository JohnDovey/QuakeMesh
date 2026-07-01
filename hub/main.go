// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.3 - Phase 2: CLI flags wired into hubapp.Hub; runs the OGM
//           engine and loopback management API until SIGINT/SIGTERM.
//   0.0.6 - Phase 5: multi-hop OGM engine (no CLI changes).
//   0.0.7 - Phase 6: DTN bundle TTL flag.

// Command quakemeshhub is the stable-backbone binary: registry, routing,
// NAT relay, and Hub-to-Hub sync. See "Project Names" in /plan.md.
//
// Phase 2–6 scope: SQLite-backed registry, multi-hop OGM routing over UDP
// against a statically configured peer list, DTN store-and-forward queue, and
// the loopback management API's /ws event stream.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/JohnDovey/QuakeMesh/hub/internal/hubapp"
)

func main() {
	cfg := hubapp.DefaultConfig()

	var peers string
	flag.StringVar(&cfg.IdentityPath, "identity", "quakemeshhub.identity", "path to this hub's Ed25519 seed file (created on first run)")
	flag.StringVar(&cfg.DBPath, "db", "quakemeshhub.db", "path to the SQLite registry database")
	flag.StringVar(&cfg.OGMBindAddr, "ogm-addr", "0.0.0.0:47222", "UDP address to send/receive OGMs on")
	flag.StringVar(&peers, "peers", "", "comma-separated list of other hubs' OGM UDP addresses (host:port)")
	flag.StringVar(&cfg.ManagementAddr, "management-addr", loopbackManagementAddr, "loopback management API bind address")
	flag.DurationVar(&cfg.OGMInterval, "ogm-interval", cfg.OGMInterval, "how often to broadcast an OGM to configured peers")
	flag.DurationVar(&cfg.StaleAfter, "stale-after", cfg.StaleAfter, "mark a peer stale after this long without a received OGM")
	flag.DurationVar(&cfg.DTNTTL, "dtn-ttl", cfg.DTNTTL, "default TTL for queued DTN bundles")
	flag.StringVar(&cfg.AppSocket, "app-socket", cfg.AppSocket, "mesh-sdk daemon listen address (unix:/path or tcp:host:port; empty disables)")
	flag.StringVar(&cfg.HeartbeatAddr, "heartbeat-addr", cfg.HeartbeatAddr, "LAN HTTP bind for mesh node heartbeats (host:port; empty disables)")
	flag.StringVar(&cfg.DiscoveryBind, "discovery-bind", cfg.DiscoveryBind, "LAN UDP bind for multicast hub/node beacons (host:port; empty disables)")
	flag.Parse()

	if peers != "" {
		for _, p := range strings.Split(peers, ",") {
			if p = strings.TrimSpace(p); p != "" {
				cfg.Peers = append(cfg.Peers, p)
			}
		}
	}

	fmt.Printf("quakemeshhub %s\n", Version)

	hub, err := hubapp.New(cfg)
	if err != nil {
		log.Fatalf("quakemeshhub: %v", err)
	}
	fmt.Printf("node id: %s\n", hub.Identity.NodeID)

	if err := hub.Start(); err != nil {
		log.Fatalf("quakemeshhub: %v", err)
	}
	fmt.Printf("OGM engine listening on %s (peers: %v)\n", cfg.OGMBindAddr, cfg.Peers)
	fmt.Printf("management API listening on %s\n", cfg.ManagementAddr)
	if cfg.AppSocket != "" {
		fmt.Printf("app SDK daemon listening on %s\n", cfg.AppSocket)
	}
	if cfg.DiscoveryBind != "" {
		fmt.Printf("LAN discovery beacons on %s (multicast %s:%d)\n", cfg.DiscoveryBind, "239.255.42.99", 47223)
	}
	if cfg.HeartbeatAddr != "" {
		fmt.Printf("node heartbeat API listening on %s\n", cfg.HeartbeatAddr)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Println("shutting down...")
	if err := hub.Close(); err != nil {
		log.Printf("quakemeshhub: shutdown: %v", err)
	}
}

// loopbackManagementAddr is the Hub's default local-only management API
// address, consumed by QuakeMeshMonitor (see /monitor).
const loopbackManagementAddr = "127.0.0.1:8083"

// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.2 - Phase 1 (protocol & identity core landed in /core).
//   0.0.3 - Phase 2: SQLite registry, OGM routing engine, loopback
//           management API.
//   0.0.4 - Version bump only; Phase 3 landed in /monitor.
//   0.0.5 - Version bump only; Phase 4 landed in /android.
//   0.0.6 - Phase 5: multi-hop OGM rebroadcast, route metric, failover.
//   0.0.7 - Phase 6: DTN store-and-forward queue.
//   0.0.8 - hub_registry liveness; HubStatusChanged management events.

package main

// Version is QuakeMeshHub's release version. Bumped on every commit
// (patch), and on minor/major only when explicitly requested.
const Version = "0.0.8"

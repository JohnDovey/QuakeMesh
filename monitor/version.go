// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// Changelog:
//   0.0.1 - Initial scaffold.
//   0.0.2 - Phase 1 (protocol & identity core landed in /core).
//   0.0.4 - Phase 3: HTTP dashboard, admin auth, Hub event stream,
//           Overview + Node Map + Relay Hub list.
//   0.0.5 - Version bump only; Phase 4 landed in /android.
//   0.0.6 - Phase 5: Network Graph and Routes dashboard views.
//   0.0.7 - Phase 6: live DTN queue depth (non-expired bundles only).
//   0.0.8 - Backbone hub tracking: /api/hubs, overview hub counts.
//   0.0.9 - Phase 7: Trust Scores view and trust-coloured map markers.
//   0.0.10 - Phase 8: orphan direction hints on Node Map; timezone in times.
//   0.0.22 - Bootstrap + jQuery responsive dashboard; endorsements and manual GPS.
//   0.0.23 - Fix blank dashboard: auto-resolve hub DB path and resilient boot.
//   0.0.24 - Sync overview tables when WebSocket counts arrive before table loaders.

package main

// Version is QuakeMeshMonitor's release version. Bumped on every commit
// (patch), and on minor/major only when explicitly requested.
const Version = "0.0.24"

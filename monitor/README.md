# /monitor

`QuakeMeshMonitor` — the web-based admin and monitoring dashboard that
runs alongside a QuakeMeshHub. See "QuakeMeshMonitor" in [/plan.md](../plan.md).

## Phase 3 scope (implemented)

- HTTP server on `0.0.0.0:8082` (override with `-bind` or `QUAKEMESH_MONITOR_PORT`)
- Embedded static assets (`go:embed`) including Bootstrap, jQuery, and Leaflet.js — works offline except map tiles
- Session-based admin login (`Admin` / `test1234` on first run, forced password change)
- Login rate limiting (5 failures → 60 s lockout)
- Overview dashboard with live counts via browser `/ws`
- Node Map (Leaflet) for nodes with GPS coordinates
- Relay hub list: manual add, TCP probe, remove
- **Routes** table and **Network Graph** (vis.js) — Phase 5
- **Infrastructure** view: Wi-Fi LAN segments (`GET /api/infrastructure`), purple hexagons on Network Graph and Node Map
- Subscribes to QuakeMeshHub's loopback management API (`127.0.0.1:8083/ws`)
- Reads/writes `quakemeshhub.db` (shared with the Hub). When you run from
  `monitor/` without `-hub-db`, the Monitor auto-picks `../quakemeshhub.db`
  if that registry is newer than a local `monitor/quakemeshhub.db`.

## Run

Start a Hub first, then the Monitor pointing at the same database:

```sh
# terminal 1 — LAN discovery + heartbeat let Android nodes auto-register
cd hub && go run . -db /tmp/quakemeshhub.db

# terminal 2
cd monitor && go run . -hub-db /tmp/quakemeshhub.db
```

Open `http://localhost:8082`, sign in as `Admin` / `test1234`, and set a new password.

## Tests

```sh
cd monitor && go test ./...
```

Later phases add Network Graph, Routes, Trust Scores, History, Ban List, and HTTPS.

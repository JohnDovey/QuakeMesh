# /monitor

`QuakeMeshMonitor` — standalone Go binary, browser-based admin dashboard
for a QuakeMeshHub. Reads the Hub's loopback management API
(`127.0.0.1:8083`) for live events and the Hub's SQLite file directly
(read-only) for history. Default bind `0.0.0.0:8082`, override via
`QUAKEMESH_MONITOR_PORT`.

All frontend assets (`static/`) are embedded via `//go:embed` — no external
files, no Node.js build step, works offline.

Not yet implemented — scaffold only (Phase 3 in [/plan.md](../plan.md)).

```sh
go run .
```

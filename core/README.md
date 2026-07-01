# /core

Shared Go module: identity, wire protocol, routing engine, trust engine,
DTN store-and-forward queue, transport interface, Noise handshake, and
SQLite schema. Imported by `/hub` and, via `gomobile`-generated `.aar`
bindings, by `/android`.

| Package | Purpose | Status |
|---|---|---|
| `identity` | Ed25519 keypair + NodeID, disk persistence, Sign/Verify | implemented (Phase 1) |
| `noise` | Noise_XX per-hop link encryption (`flynn/noise`) | implemented (Phase 1) |
| `storage` | Shared SQLite schema + migrations (`modernc.org/sqlite`) | implemented (Phase 1) |
| `simnet` | Virtual simulated-network test harness (loss/latency/peer up-down) | implemented (Phase 1) |
| `wire` | Generated Go types from `/proto` (`go generate ./...`) | implemented (Phase 1) |
| `transport` | Abstract `Transport` interface plugged in by host binaries | defined |
| `routing` | BATMAN-adv-inspired OGM routing, TQ = EQ/RQ, route metric | implemented (Phase 5) |
| `trust` | 0-100 trust score: longevity, proximity, endorsements | skeleton (Phase 7) |
| `dtn` | Store-and-forward bundle queue | skeleton (Phase 6) |
| `mobile` | gomobile-bindable Node facade for Android (`FrameSink` bridge) | implemented (Phase 4) |

Run tests: `go test ./...` (from `/core` or the repo root via the `go.work`
workspace).

See [/plan.md](../plan.md) for the full design.

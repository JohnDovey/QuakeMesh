# /core

Shared Go module: identity, wire protocol, routing engine, trust engine,
DTN store-and-forward queue, transport interface, and SQLite schema.
Imported by `/hub` and, via `gomobile`-generated `.aar` bindings, by
`/android`.

| Package | Purpose | Status |
|---|---|---|
| `identity` | Ed25519 keypair + NodeID | skeleton (Phase 1) |
| `transport` | Abstract `Transport` interface plugged in by host binaries | defined |
| `routing` | BATMAN-adv-inspired OGM routing, TQ = EQ/RQ | skeleton (Phase 1/5) |
| `trust` | 0-100 trust score: longevity, proximity, endorsements | skeleton (Phase 7) |
| `dtn` | Store-and-forward bundle queue | skeleton (Phase 6) |
| `storage` | Shared SQLite schema + migrations (`modernc.org/sqlite`) | skeleton (Phase 1) |
| `simnet` | Virtual simulated-network test harness | skeleton (Phase 1) |

See [/plan.md](../plan.md) for the full design.

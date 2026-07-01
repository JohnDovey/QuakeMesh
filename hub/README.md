# /hub

`QuakeMeshHub` — the stable backbone binary. Imports [/core](../core) for
identity and storage. Phase 2 (implemented): SQLite-backed registry,
direct single-hop OGM routing over UDP against a statically configured
peer list, and a loopback management API (default `127.0.0.1:8083`)
streaming live events over `/ws`.

Not yet implemented: multi-hop rebroadcast / BATMAN-adv TQ routing
(Phase 5), Hub-to-Hub gossip sync, internet rendezvous/relay, relay hub
propagation (Phase 9).

| Package | Purpose |
|---|---|
| `internal/registry` | Typed SQLite access for `node_registry`/`routing_table` |
| `internal/ogmengine` | UDP OGM send/receive, stale-peer sweep |
| `internal/managementapi` | `/ws` WebSocket event stream (binary protobuf `ManagementEvent`) |
| `internal/hubapp` | Wires the above into one runnable `Hub`; used by `main.go` and tests |

```sh
go run . -ogm-addr=0.0.0.0:47222 -peers=127.0.0.1:47223 -management-addr=127.0.0.1:8083
```

Run tests (includes a 3-hub in-process OGM convergence integration test
in `internal/hubapp`): `go test ./...`

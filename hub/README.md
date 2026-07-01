# /hub

`QuakeMeshHub` — the stable backbone binary. Imports [/core](../core) for
identity, routing, and storage.

## Phase 5 scope (implemented)

- Multi-hop OGM rebroadcast with TTL and `last_hop_id` / `hop_count`
- BATMAN-adv-inspired TQ = EQ/RQ link quality window
- Hello liveness pings for RTT measurement
- Route metric selection (TQ + latency + hop count) via `/core/routing`
- Automatic failover when a next-hop goes stale
- Loopback management API (`127.0.0.1:8083`) streaming `/ws` events

## Run

```sh
go run . -ogm-addr=0.0.0.0:47222 -peers=127.0.0.1:47223 -management-addr=127.0.0.1:8083
```

## Tests

```sh
go test ./...
```

Includes 3-hub mesh and linear three-hop chain integration tests.

Later phases: Hub-to-Hub gossip sync, internet rendezvous/relay (Phase 9).

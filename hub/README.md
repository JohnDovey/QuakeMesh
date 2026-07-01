# /hub

`QuakeMeshHub` — the stable backbone binary. Imports [/core](../core) for
identity, routing, trust, DTN, and storage. Adds: Hub-to-Hub gossip sync,
internet rendezvous/relay server, and a loopback management API
(`127.0.0.1:8083`) that [/monitor](../monitor) reads from.

Not yet implemented — scaffold only (Phase 2 in [/plan.md](../plan.md)).

```sh
go run .
```

# Reference apps (Phase 12)

Third-party apps built only on the mesh-sdk — no wire-protocol access required.

| App | App ID | SDK pattern |
|-----|--------|-------------|
| [privatechat](privatechat/) | `net.quakemesh.privatechat` | `Send` / `Receive`, `DiscoverPeers` |
| [discuss](discuss/) | `net.quakemesh.discuss` | `Publish` / `Subscribe` |
| [sosbeacon](sosbeacon/) | `net.quakemesh.sosbeacon` | urgent `Publish` / `Subscribe` with location |

## Prerequisites

Run **QuakeMeshHub** (or QuakeMesh Android with mesh started). The local daemon API listens on `/tmp/quakemeshhub.sock` by default.

## Private chat

```bash
# Terminal A — listen for messages
go run ./apps/privatechat -listen

# Terminal B — discover peers, then send
go run ./apps/privatechat -discover
go run ./apps/privatechat -dest <peer-node-hex> -msg "hello"
```

## Discussion board

```bash
# Terminal A — subscribe to a topic
go run ./apps/discuss -topic general -listen

# Terminal B — post
go run ./apps/discuss -topic general -post "mesh bulletin test"
```

## SOS beacon

```bash
# Terminal A — listen for SOS alerts
go run ./apps/sosbeacon -listen

# Terminal B — broadcast (optionally with GPS)
go run ./apps/sosbeacon -text "injured, need help" -lat -36.85 -lon 174.76 -acc 12
```

## Full SDK smoke test

```bash
go run ./sdk/go/cmd/meshdemo -tcp 127.0.0.1:<hub-daemon-port>
```

On Android, start the mesh and tap **SDK demo** in the QuakeMesh app to exercise `HttpMeshClient` against loopback `:18084`.

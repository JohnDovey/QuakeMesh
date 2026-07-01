# /sdk

Mesh-as-a-transport SDK: wraps the local IPC API so third-party apps use
the mesh purely as a transport, never touching the wire protocol
directly. See "Application SDK and Transport-as-a-Service" in
[/plan.md](../plan.md).

| Path | What | Status |
|---|---|---|
| [/sdk/go](go) | Go module (CLI/server integrations) | Phase 10 HTTP client |
| [/sdk/kotlin](kotlin) | Kotlin AAR (`:meshsdk`, for Android apps) | Phase 10 HttpMeshClient |

Implemented in Phase 10: local JSON HTTP API on Hub Unix socket (`unix:/tmp/quakemeshhub.sock`) and Android loopback port `18084`.

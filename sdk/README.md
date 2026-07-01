# /sdk

Mesh-as-a-transport SDK: wraps the local IPC API so third-party apps use
the mesh purely as a transport, never touching the wire protocol
directly. See "Application SDK and Transport-as-a-Service" in
[/plan.md](../plan.md).

| Path | What | Status |
|---|---|---|
| [/sdk/go](go) | Go module (CLI/server integrations) | scaffold, builds |
| [/sdk/kotlin](kotlin) | Kotlin AAR (`:meshsdk`, for Android apps) | scaffold |

Not yet implemented — scaffold only (Phase 10 in [/plan.md](../plan.md)).

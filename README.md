# QuakeMesh

A self-contained, infrastructure-independent private mesh network. See
[/plan.md](plan.md) for the full design and phased delivery roadmap.
See also [/Philosophy.md](Philosophy.md) for why this project exists.

## Repository layout

| Path | What | Status |
|---|---|---|
| [/core](core) | Go mesh-core library: identity, routing, trust, DTN, transport, storage | Phase 1 complete |
| [/hub](hub) | `QuakeMeshHub` binary | Phase 5 MVP (multi-hop routing, failover) |
| [/monitor](monitor) | `QuakeMeshMonitor` binary | Phase 5 (routes + network graph) |
| [/android](android) | `QuakeMesh` Android node app | Phase 4 MVP (transports + foreground service) |
| [/proto](proto) | Shared protobuf wire schemas | scaffold, codegen verified |
| [/sdk](sdk) | Mesh-as-a-transport SDK (Go + Kotlin) | scaffold |
| [/docs](docs) | Protocol spec, SQLite schema reference, Monitor API spec | placeholder |

## Building

Go modules are tied together with a workspace (`go.work`). The repo root is
**not** a module, so `go build ./...` from the root fails. Build from inside a
module, or pass module paths:

```sh
# binaries
go build -C hub -o QuakeMeshHub .
go build -C monitor -o QuakeMeshMonitor .

# all packages in a module
go build -C core ./...
go build -C hub ./...
go build -C monitor ./...
go build -C sdk/go ./...
```

Run without producing a binary:

```sh
cd hub && go run . -db /tmp/quakemeshhub.db
cd monitor && go run . -hub-db /tmp/quakemeshhub.db
```

Android (`/android`) and the Kotlin SDK (`/sdk/kotlin`) each need the local
Android SDK/Gradle toolchain on `PATH`:

```sh
source /Volumes/JohnDovey/source-john-dovey.sh
cd android && ./android-build.sh
```

Protobuf codegen (`/proto`) needs `protoc` + `protoc-gen-go`:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
cd proto && protoc --go_out=../core/wire --go_opt=paths=source_relative *.proto
```

## Versioning

Single project-wide version, currently in [`/VERSION`](VERSION) and
mirrored in `hub/version.go`, `monitor/version.go`, and `android/app`'s
`versionName`. The patch digit is bumped on every commit; minor/major only
on request.

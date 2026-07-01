# QuakeMesh

A self-contained, infrastructure-independent private mesh network. See
[/plan.md](plan.md) for the full design and phased delivery roadmap.

## Repository layout

| Path | What | Status |
|---|---|---|
| [/core](core) | Go mesh-core library: identity, routing, trust, DTN, transport, storage | scaffold |
| [/hub](hub) | `QuakeMeshHub` binary | scaffold |
| [/monitor](monitor) | `QuakeMeshMonitor` binary | scaffold |
| [/android](android) | `QuakeMesh` Android node app | scaffold, builds (`:app:assembleDebug`) |
| [/proto](proto) | Shared protobuf wire schemas | scaffold, codegen verified |
| [/sdk](sdk) | Mesh-as-a-transport SDK (Go + Kotlin) | scaffold |
| [/docs](docs) | Protocol spec, SQLite schema reference, Monitor API spec | placeholder |

## Building

Go modules are tied together with a workspace (`go.work`):

```sh
go build ./...   # from repo root, or inside any of core/hub/monitor/sdk/go
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

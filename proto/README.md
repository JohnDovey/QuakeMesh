# /proto

Shared wire schemas for QuakeMesh, code-generated into both Go (`/core`) and
Kotlin (`/android`). See [Wire Format](../plan.md#wire-format) in the project
plan for the full field-level rationale.

| File | Covers |
|---|---|
| `frame.proto` | Outer frame header every transport carries |
| `ogm.proto` | OGM routing advertisement + neighbour Hello/liveness ping |
| `dtn.proto` | Store-and-forward bundle envelope |
| `trust.proto` | Proximity events + trust endorsements |
| `hub_sync.proto` | Hub-to-Hub gossip payload |
| `app_presence.proto` | Third-party app presence announcements |
| `ban_list.proto` | Ban proposal + per-hub verdict records |
| `relay_hub.proto` | Relay-capable hub records |
| `management.proto` | Hub → Monitor live event stream |

## Codegen

```sh
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
# protoc-gen-go installs to $(go env GOPATH)/bin (~/go/bin) — add it to PATH
# or protoc won't find the plugin:
export PATH="$PATH:$(go env GOPATH)/bin"

# Kotlin: use protobuf-gradle-plugin from /android's build.gradle.kts instead
# of invoking protoc directly for that target.

protoc \
  --go_out=../core/wire --go_opt=paths=source_relative \
  *.proto
```

Verified working in this environment: `protoc` (libprotoc 35.1, via Homebrew)
+ `protoc-gen-go` generate all nine `.pb.go` files correctly once
`$(go env GOPATH)/bin` is on `PATH`.

Generated Go code should land in `/core/wire` (not committed here); Kotlin
codegen is driven by the Gradle protobuf plugin configured in `/android`.

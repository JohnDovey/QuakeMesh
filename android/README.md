# /android

`QuakeMesh` — the Android endpoint/repeater app. Kotlin UI + transport
shims (BLE, Wi-Fi Direct/Meshrabiya, LAN) calling into `/core` via
gomobile-generated `.aar` bindings.

## Phase 4 scope (implemented)

- **Foreground service** with persistent notification (`MeshForegroundService`)
- **gomobile bind** script: `./gomobile-bind.sh` → `app/libs/meshcore.aar` (requires Android NDK)
- **`core/mobile`** Go package: identity + SQLite + `FrameSink` bridge
- **Kotlin transports**:
  - `LanUdpTransport` — multicast discovery on connected Wi-Fi LAN
  - `BleTransport` — BLE advertise/scan skeleton (QuakeMesh service UUID)
  - `WifiMeshTransport` — Meshrabiya-pattern skeleton (Local Only Hotspot + Wi-Fi Direct)
- **`MeshEngine`** coordinates node + transports
- **`StubMeshNode`** when `meshcore.aar` is absent (emulator/dev without NDK)
- **`GoMeshNode`** reflection bridge when AAR is present

## Build

```sh
source /Volumes/JohnDovey/source-john-dovey.sh   # or ~/source-john-dovey.sh

# Optional: bind the real Go mesh core (skip for emulator / SDK-only testing)
# Run these commands in a terminal on your Mac, from this directory:
#   cd /path/to/QuakeMesh/android
#   ./gomobile-bind.sh
# Requires: Go, Android NDK (via Android Studio → SDK Manager → NDK).
# Output: app/libs/meshcore.aar

./android-build.sh                # :app:assembleDebug
./android-build.sh :app:installDebug
```

Without `meshcore.aar`, the app uses **StubMeshNode** — enough for SDK demos and the loopback API on port 18084. Bind the Go core only when you need the full crypto/routing stack on a physical device.

Tap **Start mesh** in the app. A foreground notification appears while LAN/BLE/Wi-Fi transports are active.

Physical two-phone multi-hop validation is the Phase 4 demo target; emulators cannot exercise BLE or Wi-Fi Direct fully.

See Phase 5 in [/plan.md](../plan.md) for multi-hop routing and failover.

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

# Optional: bind Go core (needs NDK under $ANDROID_HOME/ndk)
./gomobile-bind.sh

./android-build.sh                # :app:assembleDebug
./android-build.sh :app:installDebug
```

Tap **Start mesh** in the app. A foreground notification appears while LAN/BLE/Wi-Fi transports are active.

Physical two-phone multi-hop validation is the Phase 4 demo target; emulators cannot exercise BLE or Wi-Fi Direct fully.

See Phase 5 in [/plan.md](../plan.md) for multi-hop routing and failover.

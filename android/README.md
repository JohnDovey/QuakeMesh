# /android

`QuakeMesh` — the Android endpoint/repeater app. Kotlin UI + transport
shims (BLE, Wi-Fi Direct/Meshrabiya, GPS) calling into `/core` via
gomobile-generated `.aar` bindings (not yet wired up). Toolchain mirrors
the `ClonesApp` reference project (see project memory): AGP 8.13.2,
Kotlin 2.0.0, Gradle 9.5.1, compileSdk/targetSdk 35, minSdk 26, JVM 17.

Not yet implemented beyond a launchable empty Activity — scaffold only
(Phase 4 in [/plan.md](../plan.md)).

```sh
source /Volumes/JohnDovey/source-john-dovey.sh
./android-build.sh                # :app:assembleDebug
./android-build.sh :app:installDebug
```

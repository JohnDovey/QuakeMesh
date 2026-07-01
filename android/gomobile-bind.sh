#!/bin/zsh
#
# gomobile-bind.sh — build meshcore.aar from /core/mobile for Android.
#
# Prerequisites:
#   go install golang.org/x/mobile/cmd/gomobile@latest
#   go install golang.org/x/mobile/cmd/gobind@latest
#   export PATH="$PATH:$(go env GOPATH)/bin"
#   Android NDK under $ANDROID_HOME/ndk/<version> (see check_ndk below)
#   gomobile init   (run once after NDK is installed)
#
# Usage:
#   ./gomobile-bind.sh
#
# Output:
#   android/app/libs/meshcore.aar

set -e

export PATH="$PATH:$(go env GOPATH)/bin"
export ANDROID_HOME="${ANDROID_HOME:-/Volumes/JohnDovey/Android/Sdk}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT/android/app/libs"
OUT_AAR="$OUT_DIR/meshcore.aar"

mkdir -p "$OUT_DIR"

check_ndk() {
    if [[ -n "$ANDROID_NDK_HOME" && -f "$ANDROID_NDK_HOME/source.properties" ]]; then
        return 0
    fi
    local ndk_root="$ANDROID_HOME/ndk"
    if [[ -d "$ndk_root" ]]; then
        local latest
        latest="$(find "$ndk_root" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort -V | tail -1)"
        if [[ -n "$latest" && -f "$latest/source.properties" ]]; then
            export ANDROID_NDK_HOME="$latest"
            return 0
        fi
    fi
    echo "Android NDK not found under $ANDROID_HOME/ndk"
    echo ""
    echo "Install with sdkmanager (after accepting licenses):"
    echo "  export ANDROID_HOME=\"$ANDROID_HOME\""
    echo "  yes | sdkmanager --sdk_root=\"\$ANDROID_HOME\" --licenses"
    echo "  sdkmanager --sdk_root=\"\$ANDROID_HOME\" \"ndk;26.1.10909125\""
    echo ""
    echo "Or in Android Studio: Settings → Android SDK → SDK Tools →"
    echo "  check \"NDK (Side by side)\" → Apply"
    echo ""
    echo "Then run once: gomobile init"
    exit 1
}

if ! command -v gomobile >/dev/null; then
    echo "gomobile not found; install with:"
    echo "  go install golang.org/x/mobile/cmd/gomobile@latest"
    echo "  go install golang.org/x/mobile/cmd/gobind@latest"
    exit 1
fi

check_ndk

if ! command -v sdkmanager >/dev/null; then
    : # optional; NDK already present
fi

echo "Binding QuakeMesh core for Android..."
echo "   ANDROID_HOME=$ANDROID_HOME"
echo "   ANDROID_NDK_HOME=$ANDROID_NDK_HOME"
cd "$ROOT/core"
gomobile bind -target=android -androidapi=26 -o "$OUT_AAR" ./mobile

echo "Wrote $OUT_AAR"

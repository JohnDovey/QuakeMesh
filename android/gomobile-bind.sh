#!/bin/zsh
#
# gomobile-bind.sh — build meshcore.aar from /core/mobile for Android.
#
# Prerequisites:
#   go install golang.org/x/mobile/cmd/gomobile@latest
#   go install golang.org/x/mobile/cmd/gobind@latest
#   export PATH="$PATH:$(go env GOPATH)/bin"
#   gomobile init
#
# Usage:
#   ./gomobile-bind.sh
#
# Output:
#   android/app/libs/meshcore.aar

set -e

export PATH="$PATH:$(go env GOPATH)/bin"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$ROOT/android/app/libs"
OUT_AAR="$OUT_DIR/meshcore.aar"

mkdir -p "$OUT_DIR"

if ! command -v gomobile >/dev/null; then
    echo "gomobile not found; install with:"
    echo "  go install golang.org/x/mobile/cmd/gomobile@latest"
    echo "  go install golang.org/x/mobile/cmd/gobind@latest"
    exit 1
fi

echo "Binding QuakeMesh core for Android..."
cd "$ROOT/core"
gomobile bind -target=android -androidapi=26 -o "$OUT_AAR" ./mobile

echo "Wrote $OUT_AAR"

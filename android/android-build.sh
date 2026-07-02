#!/bin/zsh
#
# android-build.sh - Build QuakeMesh Android from CLI (JohnDovey drive)
#
# Toolchain matches ClonesApp / VirtAnd — see /Volumes/JohnDovey/Projects/BBS/VirtBBS/docs/CLAUDE.md.
#
# Usage examples:
#   ./android-build.sh                      # :app:assembleDebug (default)
#   ./android-build.sh :app:installDebug
#   ./android-build.sh :meshapps:assembleDebug
#   ./android-build.sh :meshapps:installDebug
#   ./android-build.sh clean
#
# Make sure the JohnDovey drive is mounted.

set -e

if [[ -f ~/source-john-dovey.sh ]]; then
    source ~/source-john-dovey.sh --quiet
elif [[ -f /Volumes/JohnDovey/source-john-dovey.sh ]]; then
    source /Volumes/JohnDovey/source-john-dovey.sh --quiet
fi

export JAVA_HOME="${JAVA_HOME:-/usr/local/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home}"

cd "$(dirname "$0")"
chmod +x gradlew 2>/dev/null || true

if [ $# -eq 0 ]; then
    set -- ":app:assembleDebug"
fi

echo "Building QuakeMesh Android..."
echo "   Args: $@"
echo "   JAVA_HOME=$JAVA_HOME"
echo "   ANDROID_HOME=${ANDROID_HOME:-/Volumes/JohnDovey/Android/Sdk}"
echo ""

# Stale Kotlin compile daemons (common after Kotlin upgrades or when the project
# lives on an external volume) can throw NoClassDefFoundError on CallResult$Dying.
# Gradle falls back to in-process compile, but stopping first avoids the noise.
./gradlew --stop >/dev/null 2>&1 || true

./gradlew "$@"

echo ""
echo "Build finished"

#!/usr/bin/env bash
# Packages the built bundle as a double-clickable .streamDeckPlugin file for
# sideloading. Output: build/com.moeilijk.lhm.streamDeckPlugin
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/build/com.moeilijk.lhm.sdPlugin"
PKG="$ROOT/build/com.moeilijk.lhm.streamDeckPlugin"
[ -d "$OUT" ] || { echo "build first: scripts/macos/build-plugin.sh"; exit 1; }
rm -f "$PKG"
# A .streamDeckPlugin is a zip whose root entry is the <uuid>.sdPlugin folder.
( cd "$ROOT/build" && zip -qr "$PKG" "com.moeilijk.lhm.sdPlugin" -x '*.DS_Store' )
echo "==> packaged $PKG ($(du -h "$PKG" | cut -f1))"

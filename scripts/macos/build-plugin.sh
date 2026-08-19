#!/usr/bin/env bash
# Builds the macOS Stream Deck plugin bundle: a universal (arm64 + x86_64)
# plugin binary plus the bundled macOS companion that supplies local Mac
# sensors. Output: build/com.moeilijk.lhm.sdPlugin
set -euo pipefail
command -v go >/dev/null || { echo "go is required (brew install go)"; exit 1; }
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/build/com.moeilijk.lhm.sdPlugin"

cd "$ROOT"
echo "==> building universal plugin binary"
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o /tmp/lhm_arm64 ./cmd/lhm_streamdeck_plugin
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /tmp/lhm_amd64 ./cmd/lhm_streamdeck_plugin
lipo -create -output /tmp/lhm_universal /tmp/lhm_arm64 /tmp/lhm_amd64

echo "==> assembling bundle"
rm -rf "$OUT"; mkdir -p "$(dirname "$OUT")"
cp -R "$ROOT/com.moeilijk.lhm.sdPlugin" "$OUT"
rm -f "$OUT"/*.exe "$OUT"/lhm-bridge* "$OUT"/lhm.log 2>/dev/null || true
cp /tmp/lhm_universal "$OUT/lhm"
chmod +x "$OUT/lhm"

echo "==> building bundled macOS companion"
cd "$ROOT/mac-companion"
# cgo (mach APIs) means each arch needs a matching -arch flag
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CGO_CFLAGS="-arch arm64" CGO_LDFLAGS="-arch arm64" \
  go build -trimpath -ldflags="-s -w" -o /tmp/companion_arm64 .
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-arch x86_64" CGO_LDFLAGS="-arch x86_64" \
  go build -trimpath -ldflags="-s -w" -o /tmp/companion_amd64 .
lipo -create -output "$OUT/lhm-companion" /tmp/companion_arm64 /tmp/companion_amd64
chmod +x "$OUT/lhm-companion"

xattr -cr "$OUT" 2>/dev/null || true
echo "==> built $OUT"
lipo -info "$OUT/lhm" "$OUT/lhm-companion"

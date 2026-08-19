#!/usr/bin/env bash
# Cross-compiles moeilijk/lhm-companion for Linux x86_64 and aarch64. Upstream
# publishes amd64 binaries only, so aarch64 hosts (NVIDIA DGX Spark) need this.
set -euo pipefail
command -v go >/dev/null || { echo "go is required (brew install go)"; exit 1; }
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="$ROOT/build/lhm-companion-src"
OUT="$ROOT/build/agents"
mkdir -p "$OUT"

if [ -d "$SRC/.git" ]; then
  echo "==> updating lhm-companion source"
  git -C "$SRC" pull --quiet --ff-only || true
else
  echo "==> cloning lhm-companion source"
  git clone --depth 1 --quiet https://github.com/moeilijk/lhm-companion.git "$SRC"
fi

cd "$SRC"
for a in amd64 arm64; do
  echo "==> linux/$a"
  CGO_ENABLED=0 GOOS=linux GOARCH="$a" go build -trimpath -ldflags="-s -w" \
    -o "$OUT/lhm-companion-linux-$a" ./cmd/lhm-companion
done
ls -la "$OUT"

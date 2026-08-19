#!/usr/bin/env bash
# Installs the built bundle into the Stream Deck plugins folder and restarts
# the app. Run scripts/macos/build-plugin.sh first.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/build/com.moeilijk.lhm.sdPlugin"
DEST="$HOME/Library/Application Support/com.elgato.StreamDeck/Plugins/com.moeilijk.lhm.sdPlugin"
[ -d "$OUT" ] || { echo "build first: scripts/macos/build-plugin.sh"; exit 1; }

echo "==> quitting Stream Deck"
osascript -e 'tell application "Elgato Stream Deck" to quit' 2>/dev/null || true
for _ in $(seq 1 20); do pgrep -x "Stream Deck" >/dev/null || break; sleep 0.5; done
pkill -x "Stream Deck" 2>/dev/null || true; sleep 1

echo "==> installing to $DEST"
rm -rf "$DEST"; mkdir -p "$(dirname "$DEST")"
cp -R "$OUT" "$DEST"
xattr -cr "$DEST" 2>/dev/null || true

echo "==> starting Stream Deck"
open -a "Elgato Stream Deck"
echo "==> installed"

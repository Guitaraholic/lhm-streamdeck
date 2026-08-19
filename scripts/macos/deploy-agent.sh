#!/usr/bin/env bash
# Deploys lhm-companion to a remote Linux host (auto-detects x86_64 / aarch64).
# Usage: scripts/deploy-agent.sh user@host [port]
#   e.g. scripts/deploy-agent.sh paul@dgx-spark-01
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
TARGET="${1:?usage: deploy-agent.sh user@host [port]}"
PORT="${2:-8085}"

echo "==> detecting architecture on $TARGET"
ARCH="$(ssh -o BatchMode=yes "$TARGET" 'uname -m')"
case "$ARCH" in
  x86_64|amd64)  BIN="lhm-companion-linux-amd64" ;;
  aarch64|arm64) BIN="lhm-companion-linux-arm64" ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac
echo "    $ARCH -> $BIN"
[ -f "$ROOT/build/agents/$BIN" ] || { echo "missing $ROOT/build/agents/$BIN (run scripts/macos/build-agents.sh)"; exit 1; }

echo "==> checking GPU tooling"
if ssh -o BatchMode=yes "$TARGET" 'command -v nvidia-smi >/dev/null 2>&1'; then
  echo "    nvidia-smi found:"
  ssh "$TARGET" 'nvidia-smi --query-gpu=name,temperature.gpu,utilization.gpu --format=csv,noheader' 2>&1 | sed 's/^/      /'
else
  echo "    no nvidia-smi — CPU/memory/hwmon only"
fi

echo "==> uploading"
scp -q "$ROOT/build/agents/$BIN" "$TARGET:/tmp/lhm-companion"

echo "==> installing (sudo)"
ssh -t "$TARGET" "sudo install -m 0755 /tmp/lhm-companion /usr/local/bin/lhm-companion && rm -f /tmp/lhm-companion && \
sudo tee /etc/systemd/system/lhm-companion.service >/dev/null <<UNIT
[Unit]
Description=LHM Companion (hardware metrics for Stream Deck)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/lhm-companion -port $PORT
Restart=always
RestartSec=5
# Reads /proc, /sys/class/hwmon and runs nvidia-smi; no privileges needed.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload && sudo systemctl enable --now lhm-companion && sleep 1 && sudo systemctl is-active lhm-companion"

echo "==> verifying endpoint"
HOSTONLY="${TARGET#*@}"
if curl -sf --max-time 5 "http://$HOSTONLY:$PORT/data.json" >/dev/null; then
  echo "    OK: http://$HOSTONLY:$PORT/data.json"
else
  echo "    endpoint not reachable from this Mac — check firewall on $HOSTONLY (port $PORT)"
fi

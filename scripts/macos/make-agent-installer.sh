#!/usr/bin/env bash
# Builds a single self-extracting installer containing the lhm-companion
# binaries for both Linux architectures. Copy the result to a target machine
# by any means (scp, USB, paste) and run it there; it needs no network access.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
AGENTS="$ROOT/build/agents"
OUT="$AGENTS/install-lhm-companion.sh"

for a in amd64 arm64; do
  [ -f "$AGENTS/lhm-companion-linux-$a" ] || { echo "missing lhm-companion-linux-$a (run scripts/macos/build-agents.sh)"; exit 1; }
done

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/payload"
cp "$AGENTS/lhm-companion-linux-amd64" "$AGENTS/lhm-companion-linux-arm64" "$WORK/payload/"
tar czf "$WORK/payload.tgz" -C "$WORK/payload" .

cat > "$OUT" <<'HEADER'
#!/bin/sh
# Self-extracting installer for lhm-companion (hardware metrics for the
# Stream Deck AI Lab Monitor). Contains binaries for x86_64 and aarch64.
#
#   sudo sh install-lhm-companion.sh                 install and start on :8085
#   sudo sh install-lhm-companion.sh --port 9000     use a different port
#   sudo sh install-lhm-companion.sh --open-firewall also open the port
#   sudo sh install-lhm-companion.sh --diagnose      report state, change nothing
#   sudo sh install-lhm-companion.sh --uninstall     stop and remove
#
# Requires no network access.
set -eu

PORT=8085
UNINSTALL=0
DIAGNOSE=0
OPEN_FW=0
while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT="$2"; shift 2 ;;
    --port=*) PORT="${1#*=}"; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    --diagnose) DIAGNOSE=1; shift ;;
    --open-firewall) OPEN_FW=1; shift ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

SERVICE=/etc/systemd/system/lhm-companion.service
BIN=/usr/local/bin/lhm-companion

if [ "$(id -u)" -ne 0 ]; then
  echo "This installer needs root. Re-run with: sudo sh $0" >&2
  exit 1
fi

# ---------------------------------------------------------------- diagnostics
listening() {
  if command -v ss >/dev/null 2>&1; then
    ss -lntp 2>/dev/null | grep -q ":$PORT "
  elif command -v netstat >/dev/null 2>&1; then
    netstat -lntp 2>/dev/null | grep -q ":$PORT "
  else
    return 1
  fi
}

firewall_hint() {
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    echo "firewalld"
  elif command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    echo "ufw"
  elif command -v nft >/dev/null 2>&1 && nft list ruleset 2>/dev/null | grep -q 'policy drop'; then
    echo "nftables"
  else
    echo ""
  fi
}

open_firewall() {
  fw="$(firewall_hint)"
  case "$fw" in
    firewalld)
      echo "==> opening $PORT/tcp in firewalld"
      firewall-cmd --permanent --add-port="$PORT"/tcp >/dev/null && firewall-cmd --reload >/dev/null
      ;;
    ufw)
      echo "==> opening $PORT/tcp in ufw"
      ufw allow "$PORT"/tcp >/dev/null
      ;;
    nftables)
      echo "!! nftables has a drop policy. Add a rule allowing tcp dport $PORT yourself." ;;
    *) echo "==> no active firewall detected" ;;
  esac
}

report() {
  echo "---------------------------------------------------------------"
  echo "diagnostics"
  echo "  arch:        $(uname -m)"
  echo "  os:          $( . /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || echo unknown )"
  if command -v systemctl >/dev/null 2>&1; then
    echo "  systemd:     $(systemctl --version 2>/dev/null | head -1)"
    echo "  unit state:  $(systemctl is-enabled lhm-companion 2>/dev/null || echo n/a) / $(systemctl is-active lhm-companion 2>/dev/null || echo inactive)"
  else
    echo "  systemd:     not present"
  fi
  echo "  binary:      $( [ -x "$BIN" ] && echo "$BIN" || echo MISSING )"
  [ -x "$BIN" ] && echo "  runs:        $("$BIN" -version 2>&1 | head -1)"
  if command -v getenforce >/dev/null 2>&1; then
    echo "  selinux:     $(getenforce 2>/dev/null)"
    command -v ls >/dev/null && [ -x "$BIN" ] && echo "  binary ctx:  $(ls -Z "$BIN" 2>/dev/null | awk '{print $1}')"
  fi
  echo "  listening:   $( listening && echo "yes (:$PORT)" || echo "NO" )"
  echo "  firewall:    $( fw=$(firewall_hint); [ -n "$fw" ] && echo "$fw active" || echo "none detected" )"
  if command -v curl >/dev/null 2>&1; then
    echo "  local fetch: $(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "http://127.0.0.1:$PORT/data.json" 2>/dev/null || echo failed)"
  fi
  if command -v systemctl >/dev/null 2>&1; then
    echo "--- last log lines ---"
    journalctl -u lhm-companion -n 20 --no-pager 2>/dev/null || systemctl status lhm-companion --no-pager -l 2>/dev/null | tail -20
  fi
  echo "---------------------------------------------------------------"
}

if [ "$DIAGNOSE" -eq 1 ]; then
  report
  exit 0
fi

if [ "$UNINSTALL" -eq 1 ]; then
  echo "==> removing lhm-companion"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl disable --now lhm-companion 2>/dev/null || true
  fi
  rm -f "$SERVICE" "$BIN"
  command -v systemctl >/dev/null 2>&1 && systemctl daemon-reload || true
  echo "==> removed"
  exit 0
fi

# ------------------------------------------------------------------- install
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  SRC=lhm-companion-linux-amd64 ;;
  aarch64|arm64) SRC=lhm-companion-linux-arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
echo "==> architecture $ARCH -> $SRC"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

START=$(awk '/^__PAYLOAD_BELOW__$/ { print NR + 1; exit 0 }' "$0")
tail -n +"$START" "$0" | base64 -d | tar xz -C "$TMP"
[ -f "$TMP/$SRC" ] || { echo "payload missing $SRC" >&2; exit 1; }

if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet lhm-companion 2>/dev/null; then
  echo "==> stopping running service"
  systemctl stop lhm-companion
fi

echo "==> installing $BIN"
install -m 0755 "$TMP/$SRC" "$BIN"

# SELinux: a binary copied out of /tmp inherits a label systemd refuses to
# execute (status=203/EXEC). Relabel it to the policy default for /usr/local/bin.
if command -v getenforce >/dev/null 2>&1 && [ "$(getenforce 2>/dev/null)" != "Disabled" ]; then
  echo "==> SELinux is $(getenforce): relabelling $BIN"
  if command -v restorecon >/dev/null 2>&1; then
    restorecon -F "$BIN" 2>/dev/null || true
  fi
  if command -v chcon >/dev/null 2>&1; then
    chcon -t bin_t "$BIN" 2>/dev/null || true
  fi
fi

echo "==> smoke test"
if ! "$BIN" -version >/dev/null 2>&1; then
  echo "!! the binary will not execute on this host:"
  "$BIN" -version || true
  exit 1
fi
echo "    $("$BIN" -version 2>&1 | head -1)"

if command -v nvidia-smi >/dev/null 2>&1; then
  echo "==> nvidia-smi found:"
  nvidia-smi --query-gpu=name,temperature.gpu,utilization.gpu --format=csv,noheader 2>&1 | sed 's/^/      /'
else
  echo "==> no nvidia-smi; CPU, memory and hwmon sensors only"
fi

if ! command -v systemctl >/dev/null 2>&1; then
  echo "==> no systemd here. Start it yourself with:"
  echo "      $BIN -port $PORT"
  exit 0
fi

# ProtectSystem=strict needs systemd 232; older releases (RHEL/CentOS 7 ship
# 219) reject the unit outright, so fall back to a directive they understand.
SDVER="$(systemctl --version 2>/dev/null | head -1 | awk '{print $2}' | tr -cd '0-9')"
[ -z "$SDVER" ] && SDVER=0
if [ "$SDVER" -ge 232 ]; then
  PROTECT="ProtectSystem=strict"
else
  PROTECT="ProtectSystem=full"
fi
echo "==> systemd $SDVER -> $PROTECT"

echo "==> writing $SERVICE (port $PORT)"
cat > "$SERVICE" <<UNIT
[Unit]
Description=LHM Companion (hardware metrics for Stream Deck)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$BIN -port $PORT
Restart=always
RestartSec=5
# Reads /proc, /sys/class/hwmon and runs nvidia-smi; no privileges needed.
NoNewPrivileges=true
$PROTECT
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
if ! systemctl enable --now lhm-companion 2>/tmp/lhm-enable.err; then
  echo "!! failed to start the service:"
  cat /tmp/lhm-enable.err
  report
  exit 1
fi

i=0
while [ $i -lt 20 ]; do
  listening && break
  i=$((i + 1))
  sleep 0.25
done

if [ "$OPEN_FW" -eq 1 ]; then
  open_firewall
fi

STATE="$(systemctl is-active lhm-companion 2>/dev/null || echo inactive)"
echo "==> service: $STATE"

if [ "$STATE" != "active" ] || ! listening; then
  echo "!! the service is not serving on :$PORT"
  report
  exit 1
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -z "$IP" ] && IP="$(hostname 2>/dev/null)"
echo "==> serving http://$IP:$PORT/data.json"

FW="$(firewall_hint)"
if [ -n "$FW" ] && [ "$OPEN_FW" -ne 1 ]; then
  echo ""
  echo "!! $FW is active and will likely block port $PORT from your Mac."
  case "$FW" in
    firewalld) echo "   Fix:  sudo firewall-cmd --permanent --add-port=$PORT/tcp && sudo firewall-cmd --reload" ;;
    ufw)       echo "   Fix:  sudo ufw allow $PORT/tcp" ;;
    nftables)  echo "   Fix:  add an nftables rule allowing tcp dport $PORT" ;;
  esac
  echo "   Or re-run this installer with --open-firewall"
fi

echo "==> done. Add this host in the Stream Deck plugin's Settings key."
exit 0
__PAYLOAD_BELOW__
HEADER

base64 < "$WORK/payload.tgz" | tr -d '\n' | fold -w 76 >> "$OUT"
echo "" >> "$OUT"
chmod +x "$OUT"
echo "built $OUT ($(du -h "$OUT" | cut -f1))"

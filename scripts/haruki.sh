#!/usr/bin/env bash
set -Eeuo pipefail
# shellcheck source=/dev/null
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

SERVICE="haruki"
WORKDIR="${BOT_HOME}/haruki"
PIDFILE="${PID_DIR}/${SERVICE}.pid"
LOGFILE="${LOG_DIR}/${SERVICE}.log"

if [[ "${1:-}" == "--daemon" ]]; then
  shift
  start_daemon_self "$SERVICE" "$PIDFILE" "$LOGFILE" -- "$@"
  exit 0
elif [[ "${1:-}" == "--stop" ]]; then
  stop_daemon "$SERVICE" "$PIDFILE"
  exit 0
elif [[ "${1:-}" == "--status" ]]; then
  status_daemon "$SERVICE" "$PIDFILE" "$LOGFILE"
  exit 0
elif [[ "${1:-}" == "--foreground" ]]; then
  shift
fi

cd "$WORKDIR"

# 允许你通过环境变量指定具体文件名（可选）
# 例如：HARUKI_BIN=HarukiClient-Linux-amd64-v1.1.7-glibc.app ./haruki.sh
BIN="${HARUKI_BIN:-}"

# 默认自动匹配 HarukiClient-Linux-*.app
if [[ -z "$BIN" ]]; then
  BIN="$(ls -1 haruki.app 2>/dev/null | head -n1 || true)"
fi

if [[ -z "$BIN" || ! -f "$BIN" ]]; then
  echo "No haruki.app found in: $WORKDIR"
  echo "Please put your file here, e.g.: haruki.app"
  exit 1
fi

# 不建议 chmod 777；只需要可执行位即可
chmod +x "$BIN"

exec "./$BIN"

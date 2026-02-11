#!/usr/bin/env bash
set -Eeuo pipefail
# shellcheck source=/dev/null
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

SERVICE="haruki_api"
WORKDIR="${BOT_HOME}/haruki-tool/Haruki-Sekai-API"
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
exec go run .

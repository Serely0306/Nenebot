#!/usr/bin/env bash
set -Eeuo pipefail
# shellcheck source=/dev/null
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

SERVICE="eventtracker"
WORKDIR="${BOT_HOME}/lunabot"
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

activate_py_env
cd "$WORKDIR"
exec python3 -m src.services.event_tracker.main
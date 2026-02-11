#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
[[ -f "$SCRIPT_DIR/env.sh" ]] && source "$SCRIPT_DIR/env.sh"

mkdir -p "$LOG_DIR" "$PID_DIR"

is_running() {
  local pidfile="$1"
  [[ -f "$pidfile" ]] || return 1
  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  [[ -n "$pid" ]] || return 1
  kill -0 "$pid" 2>/dev/null
}

start_daemon_self() {
  # Usage: start_daemon_self <service_name> <pidfile> <logfile> [-- <extra args passed to child>]
  local service="$1"; shift
  local pidfile="$1"; shift
  local logfile="$1"; shift

  if is_running "$pidfile"; then
    echo "[$service] already running (pid $(cat "$pidfile"))."
    echo "log: $logfile"
    return 0
  fi

  # Start *this script* in the background in a way that survives SSH disconnect.
  # - nohup ignores SIGHUP
  # - stdout/stderr go to logfile
  nohup "$0" --foreground "$@" >>"$logfile" 2>&1 &
  echo $! > "$pidfile"
  echo "[$service] started in background (pid $!)."
  echo "log: $logfile"
}

stop_daemon() {
  # Usage: stop_daemon <service_name> <pidfile>
  local service="$1"; shift
  local pidfile="$1"; shift

  if ! [[ -f "$pidfile" ]]; then
    echo "[$service] not running (no pidfile)."
    return 0
  fi

  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  if [[ -z "${pid:-}" ]]; then
    rm -f "$pidfile"
    echo "[$service] pidfile empty; cleaned."
    return 0
  fi

  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    echo "[$service] sent SIGTERM to pid $pid."
  else
    echo "[$service] pid $pid not alive; cleaning pidfile."
  fi
  rm -f "$pidfile"
}

status_daemon() {
  # Usage: status_daemon <service_name> <pidfile> <logfile>
  local service="$1"; shift
  local pidfile="$1"; shift
  local logfile="$1"; shift

  if is_running "$pidfile"; then
    echo "[$service] running (pid $(cat "$pidfile"))."
    echo "log: $logfile"
  else
    echo "[$service] not running."
    echo "log: $logfile"
  fi
}

activate_py_env() {
  # Prefer venv; fallback to conda env named "lunabot" if available.
  if [[ -f "$VENV_DIR/bin/activate" ]]; then
    # shellcheck source=/dev/null
    source "$VENV_DIR/bin/activate"
    return 0
  fi

  if command -v conda >/dev/null 2>&1; then
    # shellcheck source=/dev/null
    source "$(conda info --base)/etc/profile.d/conda.sh"
    conda activate lunabot
    return 0
  fi

  echo "Python env not found."
  echo "Tried: $VENV_DIR/bin/activate"
  echo "Also tried conda env: lunabot"
  return 1
}

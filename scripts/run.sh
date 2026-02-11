#!/usr/bin/env bash
set -Eeuo pipefail
# shellcheck source=/dev/null
source "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/common.sh"

SERVICE="zerobot_main"
WORKDIR="${BOT_HOME}/ZeroBot-Plugin-master"
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

if [[ ! -f "main.go" ]]; then
  echo "ERROR: main.go not found in $WORKDIR"
  exit 1
fi

echo "Checking Go installation..."
go version

echo "Setting up Go environment..."
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GO111MODULE=auto

echo "Updating dependencies..."
go mod tidy || echo "WARNING: go mod tidy had some issues, continuing..."

echo "Generating code..."
go generate main.go 2>/dev/null || echo "WARNING: go generate had some issues, continuing..."

echo "Starting ZeroBot..."
exec go run -ldflags "-s -w" main.go -c config.json

#!/usr/bin/env bash
# ===== Paths you likely need to change on the Linux server =====
export BOT_HOME="${BOT_HOME:-/root/bot}"

# Python virtualenv root
export VENV_DIR="${VENV_DIR:-$HOME/luna}"

# Logs & pid files
export LOG_DIR="${LOG_DIR:-$HOME/bot_logs}"
export PID_DIR="${PID_DIR:-$HOME/bot_pids}"

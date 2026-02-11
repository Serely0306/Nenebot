#!/usr/bin/env bash
set -Eeuo pipefail

SESSION="bots"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing: $1"; exit 1; }; }
need tmux

# 如果 session 不存在就创建
if ! tmux has-session -t "$SESSION" 2>/dev/null; then
  tmux new-session -d -s "$SESSION" -n "main"
fi

# 统一在窗口里执行：进入脚本目录再运行
neww() {
  local name="$1"; shift
  local cmd="$*"
  # 若同名窗口已存在就跳过，避免重复开
  if tmux list-windows -t "$SESSION" -F '#{window_name}' | grep -qx "$name"; then
    return 0
  fi
  tmux new-window -t "$SESSION" -n "$name" "bash -lc 'cd \"$SCRIPT_DIR\" && $cmd'"
}

# === 你要跑的服务（按需删减/改顺序）===
neww "luna"   "./luna.sh"
neww "proxy"  "./proxy.sh"
neww "deck"   "./deck.sh"
neww "api"    "./api.sh"
neww "filter" "./filter.sh"
neww "zerobot" "./run.sh"
neww "haruki" "./haruki.sh"
neww "eventtracker" "./tracker.sh"

echo "tmux session: $SESSION"
echo "attach: tmux attach -t $SESSION"
echo "detach: Ctrl+b then d"

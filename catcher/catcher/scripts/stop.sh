#!/bin/sh
# 停止 LunaBot Catcher 并清理代理设置

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

# 停止进程
pkill -f "Catcher"

# 清除代理设置
settings put global http_proxy :0

echo "已停止 LunaBot Catcher 并清除代理设置"

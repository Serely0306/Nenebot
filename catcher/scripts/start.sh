#!/bin/sh
# LunaBot Catcher 启动脚本 (Android)
# 
# 使用方法:
# 1. 将此脚本和编译好的二进制文件放到 /data/local/tmp/lunabot-catcher/
# 2. 使用 MT 管理器执行此脚本 (需要 Root 权限)

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

# 统一复用带版本校验和配置保护的部署逻辑。
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
exec "$SCRIPT_DIR/catcher.sh" "$@"

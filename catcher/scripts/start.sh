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

# 设置工作目录
ROOT_DIR="/data/local/tmp/catcher"
BIN_NAME="Catcher-android-arm64"
CONFIG_NAME="config.yaml"

# 确保目录存在
if [ ! -d "$ROOT_DIR" ]; then
    mkdir -p "$ROOT_DIR"
fi

cd "$ROOT_DIR"

# 检查二进制文件是否存在
if [ ! -f "$BIN_NAME" ]; then
    echo "错误: 未找到 $BIN_NAME"
    echo "请先将编译好的二进制文件推送到 $ROOT_DIR/"
    exit 1
fi

# 确保有执行权限
chmod 755 "$BIN_NAME"

# 设置 DNS (某些虚拟机需要)
# if [ ! -f /etc/resolv.conf ]; then
#     mount -o remount,rw /system
#     echo "nameserver 223.5.5.5" > /etc/resolv.conf
#     echo "nameserver 223.6.6.6" >> /etc/resolv.conf
# fi

# 运行程序
echo "正在启动 LunaBot Catcher..."
./"$BIN_NAME" -config "$CONFIG_NAME"

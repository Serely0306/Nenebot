#!/bin/sh
# ============================================================
# LunaBot Catcher 自动部署脚本
# 用于在 Android 设备/模拟器上自动下载并运行抓包工具
# ============================================================

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    elif command -v sudo >/dev/null 2>&1; then
        exec sudo "$0" "$@"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

# ============================================================
# 配置区域 (根据需要修改)
# ============================================================

# 工作目录
ROOT_DIR="/data/local/tmp/lunabot-catcher"

# 可执行文件名和下载地址
# TODO: 替换为你的实际下载地址
BIN_NAME="Catcher-android-arm64"
BIN_URL="https://你的服务器地址/download/$BIN_NAME"

# 配置文件名和下载地址
CONFIG_NAME="config-android.yaml"
CONFIG_URL="https://你的服务器地址/download/$CONFIG_NAME"

# 默认留空自动生成新证书
EXTERNAL_CERT_PATH=""
EXTERNAL_KEY_PATH=""

# 外部证书路径 (如果使用过HarukiProxy服务则去掉注释使用 HarukiProxy 的证书)
# EXTERNAL_CERT_PATH="/data/local/tmp/harukiproxy/ca.pem"
# EXTERNAL_KEY_PATH="/data/local/tmp/harukiproxy/ca.key"

# ============================================================
# 脚本逻辑
# ============================================================

echo "=================================================="
echo "  LunaBot Catcher 自动部署脚本"
echo "=================================================="

# 确保工作目录存在
if [ ! -d "$ROOT_DIR" ]; then
    echo "[1/4] 创建工作目录: $ROOT_DIR"
    mkdir -p "$ROOT_DIR"
fi

cd "$ROOT_DIR"

# 下载可执行文件 (如果不存在)
if [ ! -f "$BIN_NAME" ]; then
    echo "[2/4] 下载可执行文件..."
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$BIN_NAME" "$BIN_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$BIN_NAME" "$BIN_URL"
    else
        echo "错误: 未找到 curl 或 wget，无法下载文件"
        echo "请手动将 $BIN_NAME 推送到 $ROOT_DIR/"
        exit 1
    fi
    chmod 0755 "$BIN_NAME"
else
    echo "[2/4] 可执行文件已存在，跳过下载"
fi

# 下载配置文件 (如果不存在)
if [ ! -f "$CONFIG_NAME" ]; then
    echo "[3/4] 下载配置文件..."
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$CONFIG_NAME" "$CONFIG_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$CONFIG_NAME" "$CONFIG_URL"
    else
        echo "警告: 无法下载配置文件，将使用默认配置"
    fi
else
    echo "[3/4] 配置文件已存在，跳过下载"
fi

# 设置 DNS (某些虚拟机需要)
echo "[4/4] 配置网络环境..."
if [ ! -f /etc/resolv.conf ]; then
    mount -o remount,rw /system 2>/dev/null
    touch /etc/resolv.conf 2>/dev/null
fi
if ! grep -q "223.5.5.5" /etc/resolv.conf 2>/dev/null; then
    mount -o remount,rw /system 2>/dev/null
    echo "nameserver 223.5.5.5" >> /etc/resolv.conf 2>/dev/null
    echo "nameserver 223.6.6.6" >> /etc/resolv.conf 2>/dev/null
fi

echo ""
echo "=================================================="
echo "  准备完成，正在启动 LunaBot Catcher..."
echo "=================================================="
echo ""

# 运行程序
exec ./"$BIN_NAME" -config "$CONFIG_NAME"

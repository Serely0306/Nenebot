#!/bin/sh
# 卸载 LunaBot Catcher

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

ROOT_DIR="/data/local/tmp/lunabot-catcher"
SYSTEM_CACERTS_DIR="/system/etc/security/cacerts"

echo "正在卸载 LunaBot Catcher..."

# 停止进程
pkill -f "Catcher-android-arm64" 2>/dev/null
pkill -f "lunabot-catcher" 2>/dev/null

# 清除代理设置
settings put global http_proxy :0 2>/dev/null
settings put global https_proxy :0 2>/dev/null

# 删除自动生成的证书 (如果有)
if [ -d "$ROOT_DIR" ]; then
    # 查找并删除系统证书目录中对应的证书
    find "$ROOT_DIR" -type f -name "*.0" 2>/dev/null | while read cacert_file; do
        file_name=$(basename "$cacert_file")
        system_cacert_file="$SYSTEM_CACERTS_DIR/$file_name"
        if [ -f "$system_cacert_file" ]; then
            echo "删除证书: $system_cacert_file"
            mount -o remount,rw /system 2>/dev/null
            rm -f "$system_cacert_file" 2>/dev/null
        fi
    done
    
    # 删除工作目录
    echo "删除目录: $ROOT_DIR"
    rm -rf "$ROOT_DIR"
fi

echo "卸载完成！"

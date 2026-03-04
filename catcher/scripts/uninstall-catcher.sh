#!/bin/sh

# LunaBot Catcher 卸载脚本
# 清理: 系统证书、代理设置、DNS配置、catcher目录

# 提权检查
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    elif command -v sudo >/dev/null 2>&1; then
        exec sudo "$0" "$@"
    else
        echo "此脚本需要 root 权限运行" >&2
        exit 1
    fi
fi

CATCHER_DIR="/data/local/tmp/catcher"
SYSTEM_CACERTS_DIR="/system/etc/security/cacerts"
CERT_FILE="$CATCHER_DIR/ca-cert.pem"

echo "========== LunaBot Catcher 卸载 =========="

# 1. 清除系统代理
echo "[1/4] 清除系统代理..."
settings put global http_proxy :0 2>/dev/null
settings put global global_http_proxy_host "" 2>/dev/null
settings put global global_http_proxy_port 0 2>/dev/null
echo "  ✓ 代理已清除"

# 2. 删除系统证书
echo "[2/4] 删除系统证书..."
cert_removed=false

# 方法1: 从 ca-cert.pem 计算哈希值找到对应的 .0 文件
if [ -f "$CERT_FILE" ]; then
    if command -v openssl >/dev/null 2>&1; then
        CERT_HASH=$(openssl x509 -subject_hash_old -noout -in "$CERT_FILE" 2>/dev/null)
        if [ -n "$CERT_HASH" ]; then
            CERT_NAME="${CERT_HASH}.0"
            SYSTEM_CERT="$SYSTEM_CACERTS_DIR/$CERT_NAME"
            if [ -f "$SYSTEM_CERT" ]; then
                mount -o remount,rw /system 2>/dev/null
                rm -f "$SYSTEM_CERT"
                echo "  ✓ 已删除证书: $SYSTEM_CERT"
                cert_removed=true
            else
                echo "  - 证书不存在: $SYSTEM_CERT"
            fi
        fi
    fi
fi

# 方法2: 如果没有 openssl 或 ca-cert.pem，尝试通过证书内容匹配
if [ "$cert_removed" = false ] && [ -f "$CERT_FILE" ]; then
    mount -o remount,rw /system 2>/dev/null
    for sys_cert in "$SYSTEM_CACERTS_DIR"/*.0; do
        [ -f "$sys_cert" ] || continue
        # 比较证书指纹
        if diff -q "$CERT_FILE" "$sys_cert" >/dev/null 2>&1; then
            rm -f "$sys_cert"
            echo "  ✓ 已删除证书 (内容匹配): $sys_cert"
            cert_removed=true
            break
        fi
    done
fi

# 方法3: 清理旧版固定名称的证书
OLD_CERT="$SYSTEM_CACERTS_DIR/lunabot-catcher.0"
if [ -f "$OLD_CERT" ]; then
    mount -o remount,rw /system 2>/dev/null
    rm -f "$OLD_CERT"
    echo "  ✓ 已删除旧证书: $OLD_CERT"
    cert_removed=true
fi


if [ "$cert_removed" = false ]; then
    echo "  - 未找到需要删除的证书"
fi

# 3. 删除 DNS 配置
echo "[3/4] 清理 DNS 配置..."
if [ -f /etc/resolv.conf ]; then
    mount -o remount,rw /system 2>/dev/null
    rm -f /etc/resolv.conf
    echo "  ✓ 已删除 /etc/resolv.conf"
else
    echo "  - resolv.conf 不存在，跳过"
fi

# 4. 删除 catcher 目录
echo "[4/4] 删除 catcher 目录..."
if [ -d "$CATCHER_DIR" ]; then
    rm -rf "$CATCHER_DIR"
    echo "  ✓ 已删除: $CATCHER_DIR"
else
    echo "  - 目录不存在: $CATCHER_DIR"
fi

# 同时清理旧路径
OLD_DIR="/data/local/catcher"
if [ -d "$OLD_DIR" ]; then
    rm -rf "$OLD_DIR"
    echo "  ✓ 已删除旧目录: $OLD_DIR"
fi

echo ""
echo "========== 卸载完成 =========="
echo "建议重启设备使证书删除生效"

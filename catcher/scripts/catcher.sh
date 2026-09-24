#!/bin/sh
set -u

# 配置区：只由此启动脚本读取，修改后下次启动生效。
# 工作目录，保存二进制、实际配置和配置示例。
ROOT_DIR="/data/local/tmp/catcher"
# 服务端下载接口和二进制文件名。
BASE_URL="https://159.75.213.84/upload/download"
BIN_NAME="Catcher-android-arm64"
# 1：启动时检查并更新二进制；0：已有二进制时不联网检查。
# 无论开关如何，首次安装或二进制为空时均下载并校验。
AUTO_UPDATE_BINARY=1
# 1：启动时刷新配置示例；0：仅缺少实际配置和示例时下载。
# 此开关独立于二进制更新，已有实际配置始终不覆盖。
AUTO_UPDATE_CONFIG_EXAMPLE=1
CONFIG_NAME="config-android.yaml"
CONFIG_EXAMPLE_NAME="config-android.yaml.example"
# 下载超时（秒），避免离线时无限等待。
DOWNLOAD_TIMEOUT=60

case "$AUTO_UPDATE_BINARY:$AUTO_UPDATE_CONFIG_EXAMPLE" in
    0:0|0:1|1:0|1:1) ;;
    *) echo "自动更新开关必须为 0 或 1" >&2; exit 1 ;;
esac

if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then exec su -c "$0 $*"; fi
    echo "此脚本需要 Root 权限运行" >&2
    exit 1
fi

mkdir -p "$ROOT_DIR" || exit 1
cd "$ROOT_DIR" || exit 1

download() {
    download_dest="$1"; download_url="$2"; download_tmp="$1.tmp.$$"
    rm -f "$download_tmp"
    if command -v curl >/dev/null 2>&1; then
        curl -k -fsSL --connect-timeout 10 --max-time "$DOWNLOAD_TIMEOUT" -o "$download_tmp" "$download_url" || {
            rm -f "$download_tmp"; return 1;
        }
    elif command -v wget >/dev/null 2>&1; then
        wget --no-check-certificate -T "$DOWNLOAD_TIMEOUT" -q -O "$download_tmp" "$download_url" || {
            rm -f "$download_tmp"; return 1;
        }
    else echo "缺少 curl 或 wget" >&2; return 1; fi
    [ -s "$download_tmp" ] || { rm -f "$download_tmp"; return 1; }
    mv -f "$download_tmp" "$download_dest"
}

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
    elif command -v sha256 >/dev/null 2>&1; then sha256 -q "$1"
    elif command -v openssl >/dev/null 2>&1; then openssl dgst -sha256 "$1" | awk '{print $NF}'
    else return 1; fi
}

BIN_URL="$BASE_URL/$BIN_NAME"
hash_file=".catcher-hash.$$"
candidate=".catcher-binary.$$"
trap 'rm -f "$hash_file" "$candidate" "$hash_file.tmp.$$" "$candidate.tmp.$$" "$CONFIG_EXAMPLE_NAME.tmp.$$"' EXIT
trap 'exit 1' HUP INT TERM

update_binary() {
    # 校验值由服务端根据当前文件实时计算，无需发布或维护清单。
    download "$hash_file" "$BIN_URL?sha256=1" || return 1
    remote_hash=$(tr -d '\r\n' < "$hash_file" | tr 'A-F' 'a-f')
    case "$remote_hash" in
        ''|*[!0-9a-f]*) echo "服务端 SHA256 无效" >&2; return 1 ;;
    esac
    [ "${#remote_hash}" -eq 64 ] || { echo "服务端 SHA256 长度无效" >&2; return 1; }
    local_hash=$(sha256 "$BIN_NAME" 2>/dev/null || true)
    if [ "$local_hash" = "$remote_hash" ]; then
        echo "二进制与服务端一致，跳过下载"
        return 0
    fi
    echo "正在下载/更新 $BIN_NAME..."
    download "$candidate" "$BIN_URL" || return 1
    candidate_hash=$(sha256 "$candidate" 2>/dev/null || true)
    [ "$candidate_hash" = "$remote_hash" ] || { echo "二进制校验失败，保留原文件" >&2; return 1; }
    chmod 0755 "$candidate" || return 1
    mv -f "$candidate" "$BIN_NAME"
}

if [ ! -s "$BIN_NAME" ] || [ "$AUTO_UPDATE_BINARY" -eq 1 ]; then
    if ! update_binary; then
        if [ -s "$BIN_NAME" ]; then
            echo "更新检查或下载失败，继续使用原二进制" >&2
        else
            echo "首次安装失败，无法启动" >&2
            exit 1
        fi
    fi
else
    echo "二进制自动更新已关闭，使用本地文件"
fi
rm -f "$hash_file" "$candidate"
chmod 0755 "$BIN_NAME" || exit 1

if [ "$AUTO_UPDATE_CONFIG_EXAMPLE" -eq 1 ] || { [ ! -f "$CONFIG_NAME" ] && [ ! -s "$CONFIG_EXAMPLE_NAME" ]; }; then
    if download "$CONFIG_EXAMPLE_NAME" "$BASE_URL/$CONFIG_EXAMPLE_NAME"; then
        echo "配置示例已更新: $CONFIG_EXAMPLE_NAME"
    else
        echo "配置示例下载失败，保留本地配置" >&2
    fi
fi
if [ ! -f "$CONFIG_NAME" ] && [ -s "$CONFIG_EXAMPLE_NAME" ]; then
    cp "$CONFIG_EXAMPLE_NAME" "$CONFIG_NAME" || exit 1
    echo "已创建初始配置: $CONFIG_NAME"
fi
if [ ! -f "$CONFIG_NAME" ]; then
    echo "错误: 缺少配置文件 $CONFIG_NAME" >&2
    exit 1
fi

if [ ! -f /etc/resolv.conf ]; then
    mount -o remount,rw /system 2>/dev/null
    touch /etc/resolv.conf 2>/dev/null
fi
if ! grep -q "223.5.5.5" /etc/resolv.conf 2>/dev/null; then
    mount -o remount,rw /system 2>/dev/null
    echo "nameserver 223.5.5.5" >> /etc/resolv.conf 2>/dev/null
    echo "nameserver 223.6.6.6" >> /etc/resolv.conf 2>/dev/null
fi

exec "./$BIN_NAME" -config "$CONFIG_NAME"

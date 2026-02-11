# LunaBot Catcher Android 使用教程

——世界计划 MySekai 数据自动抓包工具

**软件作者：LunaBot**

---

## 目录

1. [前言](#前言)
2. [准备工作](#准备工作)
3. [虚拟机设置](#虚拟机设置)
4. [LunaBot Catcher 设置](#lunabot-catcher-设置)
5. [开始抓包](#开始抓包)
6. [附录](#附录)

---

## 前言

本教程将指导你如何在 Android 虚拟机中使用 LunaBot Catcher 自动抓取世界计划 (Project Sekai) 的 MySekai 数据，并自动上传到 LunaBot 服务器。

**功能特性：**
- ✅ 自动拦截 MySekai 全量数据请求
- ✅ 自动解密并上传到服务器
- ✅ 支持复用 HarukiProxy 证书（无需重新安装证书）
- ✅ 支持所有服务器 (JP/EN/TW/KR/CN)

**注意：** 本工具仅用于数据备份和个人使用，请勿用于任何违规用途。

---

## 准备工作

### 1. 下载所需文件

| 文件 | 说明 |
|------|------|
| 光速虚拟机 | https://app.gsxnj.cn/ |
| MT管理器 | https://mt2.cn/ |
| 世界计划游戏 | 你要抓包的游戏客户端 |

### 2. 虚拟机设置

1. 下载并安装光速虚拟机
2. 新建虚拟机，选择 **安卓7** 和 **32+64位**
3. 启动虚拟机，导入游戏和 MT管理器
4. 在虚拟机设置中开启 **超级用户（Root）**

> **提示：** 安卓12、13的华为、荣耀系统需要用电脑解锁子进程限制

---

## LunaBot Catcher 设置

### 方法一：使用自动部署脚本（推荐）

1. 打开 MT管理器
2. 点击左上角路径，跳转到 `/sdcard/Documents`
3. 点击 **"+"** → 新建文件 → 输入文件名 `catcher.sh`
4. 编辑文件，输入以下内容：

```bash
#!/bin/sh

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    exec su -c "$0 $*"
fi

ROOT_DIR="/data/local/tmp/lunabot-catcher"
BIN_NAME="Catcher-android-arm64"
CONFIG_NAME="config-android.yaml"

# TODO: 替换为实际下载地址
BIN_URL="https://你的服务器/download/$BIN_NAME"
CONFIG_URL="https://你的服务器/download/$CONFIG_NAME"

# 创建目录
mkdir -p "$ROOT_DIR"
cd "$ROOT_DIR"

# 下载文件
if [ ! -f "$BIN_NAME" ]; then
    curl -L -o "$BIN_NAME" "$BIN_URL"
    chmod 755 "$BIN_NAME"
fi

if [ ! -f "$CONFIG_NAME" ]; then
    curl -L -o "$CONFIG_NAME" "$CONFIG_URL"
fi

# 设置 DNS
if [ ! -f /etc/resolv.conf ]; then
    mount -o remount,rw /system
    echo "nameserver 223.5.5.5" > /etc/resolv.conf
fi

# 运行
exec ./"$BIN_NAME" -config "$CONFIG_NAME"
```

5. 保存文件
6. 点击 `catcher.sh` → 设置 → 选择 **"使用扩展包环境执行"** → 勾选 **"使用ROOT权限执行"**
7. 点击 **执行**

### 方法二：手动部署

1. 在电脑上编译好 `Catcher-android-arm64`
2. 使用 ADB 推送文件：

```bash
adb push Catcher-android-arm64 /data/local/tmp/lunabot-catcher/
adb push config-android.yaml /data/local/tmp/lunabot-catcher/
adb shell chmod 755 /data/local/tmp/lunabot-catcher/Catcher-android-arm64
```

3. 在虚拟机中使用 MT管理器 执行

---

## 配置文件说明

编辑 `/data/local/tmp/lunabot-catcher/config-android.yaml`：

```yaml
# 监听地址
listen: "0.0.0.0:8888"

# 上传服务器地址 (必须修改!)
upload_server: "http://你的服务器IP:5000"

# 是否自动上传
auto_upload: true

# 是否保存到本地
save_locally: true
save_dir: "./data"

# Android 代理 IP (建议设置为 127.0.0.1)
android_proxy_ip: "127.0.0.1"

# 复用 HarukiProxy 证书 (如果有)
external_cert_path: "/data/local/tmp/harukiproxy/ca.pem"
external_key_path: "/data/local/tmp/harukiproxy/ca.key"
```

**重要配置项：**

| 配置项 | 说明 |
|--------|------|
| `upload_server` | **必须修改**为你的 LunaBot 上传服务地址 |
| `android_proxy_ip` | 建议设置为 `127.0.0.1`，否则可能无法联网 |
| `external_cert_path` | 如果已安装过 HarukiProxy 证书，填写路径即可复用 |

---

## 开始抓包

1. 使用 MT管理器 执行 `catcher.sh` 脚本
2. 看到以下输出表示启动成功：

```
╔══════════════════════════════════════════════════════════════╗
║           LunaBot Catcher - 世界计划抓包工具                  ║
╠══════════════════════════════════════════════════════════════╣
║  监听地址: 0.0.0.0:8888
║  上传服务: http://xxx.xxx.xxx.xxx:5000
╚══════════════════════════════════════════════════════════════╝
[代理] MITM 代理已启动: 0.0.0.0:8888
```

3. 打开游戏，登录账号
4. 进入 **MySekai** 界面
5. 看到以下日志表示抓包成功：

```
==================================================
[拦截] cn - mysekai (全量) - UID: xxxxxxxxxx
[请求] /api/user/xxx/mysekai?isForceAllReloadOnlyMysekai=True
==================================================
[mysekai] ✓ 数据处理成功: cn user xxxxxxxxxx
```

6. 完成！数据已自动上传到服务器

---

## 附录

### 停止 LunaBot Catcher

新建 `kill-catcher.sh`：

```bash
#!/bin/sh
if [ "$(id -u)" -ne 0 ]; then
    exec su -c "$0 $*"
fi

pkill -f "Catcher-android-arm64"
settings put global http_proxy :0
echo "已停止 LunaBot Catcher"
```

### 卸载 LunaBot Catcher

新建 `uninstall-catcher.sh`：

```bash
#!/bin/sh
if [ "$(id -u)" -ne 0 ]; then
    exec su -c "$0 $*"
fi

pkill -f "Catcher-android-arm64"
settings put global http_proxy :0
rm -rf /data/local/tmp/lunabot-catcher
echo "卸载完成"
```

### 常见问题

**Q: 启动后虚拟机无法联网？**

A: 将 `config-android.yaml` 中的 `android_proxy_ip` 设置为 `127.0.0.1`

**Q: 解密失败？**

A: 检查服务器密钥是否正确，或查看保存的 `.bin` 文件进行调试

**Q: 如何复用 HarukiProxy 证书？**

A: 在配置文件中设置 `external_cert_path` 和 `external_key_path` 为 HarukiProxy 的证书路径

---

## 关于文件托管

要让脚本能够自动下载可执行文件，你需要将文件托管到一个 HTTP 服务器：

### 方案 1：使用你的 LunaBot 上传服务

在 `upload/server.py` 中添加静态文件服务：

```python
@app.route('/download/<filename>')
def download_file(filename):
    return send_from_directory('/path/to/files', filename)
```

### 方案 2：使用 Nginx 静态文件服务

```nginx
location /download/ {
    alias /path/to/files/;
    autoindex on;
}
```

### 方案 3：使用云存储

将文件上传到 OSS/COS 等云存储服务，使用公开访问链接。

---

**祝使用愉快！**

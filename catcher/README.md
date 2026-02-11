# LunaBot Catcher - Android 抓包工具

这是一个用 Go 语言编写的 Android 抓包工具，可直接在 Android 设备上运行，用于抓取世界计划 (Project Sekai) 的游戏数据并上传到 LunaBot 服务器。

## 功能特性

- ✅ 支持多区服 (JP/CN/TW/KR/EN)
- ✅ HTTPS MITM 代理
- ✅ 自动解密游戏数据 (使用 AES-CBC)
- ✅ 自动上传到 LunaBot 服务器
- ✅ 可选保存原始数据到本地
- ✅ 自动安装 CA 证书 (需要 Root)
- ✅ 自动设置 Android 系统代理

## 编译

### Windows/Linux/macOS 本地运行

```bash
go build -o catcher ./cmd/catcher
```

### Android ARM64 交叉编译

```bash
# Linux/macOS
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o Catcher-android-arm64 ./cmd/catcher

# Windows (PowerShell)
$env:GOOS="android"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"; go build -o Catcher-android-arm64 ./cmd/catcher
```

## 配置文件

创建 `config.yaml`:

```yaml
# 监听设置
listen: "0.0.0.0:8888"

# 上传服务器地址 (你的 LunaBot 上传服务)
upload_server: "http://你的服务器IP:5000"

# 是否自动上传
auto_upload: true

# 是否保存到本地
save_locally: false
save_dir: "./data"

# Android 代理 IP (留空自动检测，如果无法联网设置为 127.0.0.1)
android_proxy_ip: ""

# 调试模式
debug: false
```

## Android 使用方法

1. 将编译好的二进制文件和 `config.yaml` 推送到 Android 设备:

```bash
adb push Catcher-android-arm64 /data/local/tmp/catcher/
adb push config.yaml /data/local/tmp/catcher/
adb shell chmod 755 /data/local/tmp/catcher/Catcher-android-arm64
```

2. 使用 Root 权限运行:

```bash
adb shell
su
cd /data/local/tmp/catcher
./LunaBotCatcher-android-arm64
```

3. 工具会自动:
   - 生成并安装 CA 证书到系统目录
   - 设置系统 HTTP 代理
   - 启动 MITM 代理

4. 启动游戏，登录后数据会自动抓取并上传

5. 按 Ctrl+C 退出，会自动清理代理设置

## Shell 脚本 (可选)

你也可以创建一个启动脚本 `start.sh`:

```bash
#!/bin/sh

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    exec su -c "$0 $*"
fi

cd /data/local/tmp/catcher
./Catcher-android-arm64
```

## 注意事项

1. **需要 Root 权限**: 安装系统证书和设置全局代理需要 Root
2. **推荐虚拟机**: 在光速虚拟机等 Android 虚拟机中运行更方便
3. **国服限制**: 由于加密方式不同，国服 MySekai 可能无法抓取

## 文件结构

```
catcher/
├── cmd/
│   └── catcher/
│       └── main.go           # 主程序入口
├── internal/
│   ├── config/
│   │   └── config.go         # 配置加载
│   ├── crypto/
│   │   └── decrypt.go        # 游戏数据解密
│   ├── proxy/
│   │   └── proxy.go          # MITM 代理
│   └── uploader/
│       └── uploader.go       # 数据上传
├── go.mod
├── go.sum
├── config.yaml               # 配置文件
└── README.md
```

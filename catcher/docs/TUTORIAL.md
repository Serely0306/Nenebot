# Catcher Android 使用说明

`Catcher` 用于在 Android 设备或虚拟机中拦截 Project Sekai 的 Suite / MySekai 接口，并将结果上传到 `upload` 服务，或保存在本地目录。

## 运行前提

- 设备具备 Root 权限
- 设备能够访问 `upload` 服务地址
- 已准备好以下文件之一：
  - `Catcher-android-arm64`
  - `config-android.yaml`
  - `scripts/catcher.sh`
  - `scripts/kill-catcher.sh`
  - `scripts/uninstall-catcher.sh`

## 文件角色

- `Catcher-android-arm64`：Android ARM64 可执行文件
- `config-android.yaml`：Android 默认配置模板
- `scripts/catcher.sh`：自动下载并启动的脚本模板
- `scripts/kill-catcher.sh`：停止进程并清理常用代理设置
- `scripts/uninstall-catcher.sh`：清理代理、证书、DNS 与工作目录

## 下载地址约定

如果 `upload` 服务挂在反向代理的 `/upload` 前缀下，下载地址通常是：

```text
https://<host>/upload/download/<filename>
```

如果直接暴露 Flask 服务，则通常是：

```text
http://<host>:5000/download/<filename>
```

## 配置关键项

`config-android.yaml` 中最关键的是以下字段：

| 字段 | 当前作用 |
| --- | --- |
| `upload_server` | `upload` 服务基地址；留空时不会向服务端上传 |
| `save_locally` / `save_dir` | 是否在本地保存处理结果或原始响应 |
| `android_proxy_ip` | 代理地址；虚拟机网络异常时通常改为 `127.0.0.1` |
| `capture_suite` | 是否抓取 `/api/suite/user/{uid}` |
| `capture_mysekai` | 是否抓取 MySekai 全量响应 |
| `force_mysekai_reload` | 将 `isForceAllReloadOnlyMysekai=False` 改写为 `True`，并隐式启用 MySekai 抓取 |
| `mitm_target_only` | 仅对已知游戏 API 域名执行 MITM |
| `external_cert_path` / `external_key_path` | 复用已有证书，例如 HarukiProxy 证书 |

补充说明：

- 当前仓库内的 `config-android.yaml` 默认只启用了 `capture_suite: true`。
- 如果要抓 MySekai，建议显式增加 `force_mysekai_reload: true`。
- `auto_upload` 字段目前未参与运行逻辑，是否上传实际由 `upload_server` 是否为空决定。

## 推荐流程

### 1. 准备脚本与配置

如果直接使用仓库里的 `scripts/catcher.sh`，先把脚本中的以下变量改成真实地址：

```sh
BIN_URL="https://<your-host>/upload/download/Catcher-android-arm64"
CONFIG_URL="https://<your-host>/upload/download/config-android.yaml"
```

这两个地址在仓库版本里仍是占位符，不能直接使用。

### 2. 准备工作目录

当前脚本默认工作目录为：

```text
/data/local/tmp/lunabot-catcher
```

脚本会把二进制和配置文件下载到这个目录，并从这里启动程序。

### 3. 启动

推荐执行 `scripts/catcher.sh`。如果手动启动，可使用：

```sh
adb push Catcher-android-arm64 /data/local/tmp/lunabot-catcher/
adb push config-android.yaml /data/local/tmp/lunabot-catcher/
adb shell chmod 755 /data/local/tmp/lunabot-catcher/Catcher-android-arm64
adb shell su -c 'cd /data/local/tmp/lunabot-catcher && ./Catcher-android-arm64 -config config-android.yaml'
```

首次运行如果安装了新的系统证书，程序会尝试触发重启；重启后需要再次执行启动命令。

### 4. 抓取触发方式

- Suite：进入会触发 `/api/suite/user/{uid}` 的相关流程
- MySekai：只会保存全量响应；建议开启 `force_mysekai_reload` 后再进入 MySekai 页面

程序会在以下两种情况下保留原始二进制：

- `save_locally: true`
- 解密失败，便于回溯问题

### 5. 停止与清理

- 日常停止：执行 `scripts/kill-catcher.sh`
- 完整清理：执行 `scripts/uninstall-catcher.sh`

仅关闭终端不足以保证代理被清理。

## 常见问题

### 启动后设备无法联网

优先把 `android_proxy_ip` 改成 `127.0.0.1`。如果仍然异常，再手动检查系统或 Wi-Fi 代理是否被正确设置为 `127.0.0.1:8888`。

### 抓到了请求但没有生成可用数据

- MySekai 只处理全量请求；若未开启 `force_mysekai_reload`，可能只拿到了增量数据
- Suite 响应必须包含 `userGamedata`，否则会被视为不完整数据并跳过

### 需要复用已有证书

在配置中设置：

```yaml
external_cert_path: "/data/local/tmp/harukiproxy/ca.pem"
external_key_path: "/data/local/tmp/harukiproxy/ca.key"
```

这样会直接使用外部证书，而不是重新生成新的 CA。

# MySekai 数据上传服务

用于上传 MySekai 抓包数据到 LunaBot 的 Web 服务。支持上传游戏服务端原始二进制响应包，自动解密并保存。

## 功能

- ✅ **自动解密**: 支持上传游戏原始 .bin 响应包
- ✅ **QQ 绑定查询**: 输入 QQ 号自动查找已绑定的游戏账号
- ✅ **API 支持**: 提供 HTTP 接口供 LunaBot 获取数据
- ✅ **多区服支持**: 支持 jp/cn/tw/kr/en

---

## 🚀 完整部署指南 (Linux/Server)

本指南将帮助你在服务器上部署该服务，并配置 Nginx 反向代理。

### 1. 环境准备

确保服务器已安装 Python 3.10+ 和 pip。

```bash
# 克隆/下载项目代码到服务器
git clone <your-repo-url> mysekai-upload
cd mysekai-upload

# 安装依赖
pip install -r requirements.txt
```

### 2. 配置 Systemd 服务 (后台运行)

创建服务文件，使程序在后台稳定运行并在开机时自动启动。

创建文件 `/etc/systemd/system/mysekai-upload.service`:

```ini
[Unit]
Description=MySekai Upload Service
After=network.target

[Service]
# 修改为你的用户名和项目路径
User=root
WorkingDirectory=/root/bot/upload
# 如果使用了虚拟环境，请指向虚拟环境的 python
ExecStart=/usr/bin/python3 server.py --port 5000
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable upload
sudo systemctl start upload
sudo systemctl status upload  # 检查状态
```

### 3. 配置 Nginx 反向代理 (可选但推荐)

配置 Nginx 使其可以通过标准 HTTP 端口 (80) 访问，并作为一个子路径（如 `/mysekai_upload/`）。

编辑 Nginx 配置 (通常在 `/etc/nginx/sites-available/default` 或 `/etc/nginx/nginx.conf`):

```nginx
server {
    listen 80;
    server_name 103.236.55.101;  # 替换为你的 IP 或域名

    # 1. 网页上传界面
    location /mysekai_upload/ {
        proxy_pass http://127.0.0.1:5000/;  # 注意末尾的斜杠
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        
        # 允许大文件上传 (MySekai 数据可能较大)
        client_max_body_size 50M;
    }

    # 2. 供 LunaBot 调用的 API 接口 (如果不希望公开，可跳过此块，让 Bot 直接访问 localhost:5000)
    location /api/mysekai/ {
        proxy_pass http://127.0.0.1:5000/api/mysekai/;
    }
}
```

重载 Nginx:
```bash
sudo nginx -t
sudo systemctl reload nginx
```

现在可以通过 `http://你的IP/mysekai_upload/` 访问上传页面。

---

## ⚙️ 配置 LunaBot 访问本地数据

为了让 LunaBot 使用你上传的数据，你需要修改 `lunabot/example_config/sekai/gameapi.yaml`。

### 修改步骤

找到对应的区服配置（如 `cn`），修改 `mysekai_api_url` 字段。

**如果 LunaBot 和上传服务在同一台机器上（推荐）：**
直接指向本地端口。

```yaml
# 在 lunabot/config/sekai/gameapi.yaml 中

cn:
  # ... 其他配置保持不变 ...
  # 修改这一行，指向你的本地上传服务接口
  # 注意：{uid} 是占位符，不要替换它
  mysekai_api_url: "http://127.0.0.1:5000/api/mysekai/cn/{uid}"
```

**如果 LunaBot 在另一台机器上：**
使用上传服务的公网 IP 或域名。

```yaml
cn:
  mysekai_api_url: "http://103.236.55.101/api/mysekai/cn/{uid}"  # 如果没配 Nginx，加上 :5000
```

### 验证

1. 重启 LunaBot。
2. 在上传网页上传一份数据。
3. 在 LunaBot 发送 `/msr` 指令，Bot 应该能获取到刚刚上传的数据。

---

## 上传数据保存路径

- **Windows**: `lunabot\data\sekai\user_data\{region}\mysekai\{uid}.json`
- **Linux**: `lunabot/data/sekai/user_data/{region}/mysekai/{uid}.json`

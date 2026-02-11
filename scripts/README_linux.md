# Linux 启动脚本（从 .bat 迁移）

## 0) 放置位置建议
把整个 `linux_scripts/` 目录上传到服务器，例如：
- `~/bot_scripts/`

然后：
```bash
cd ~/bot_scripts
chmod +x *.sh
```

## 1) 先改路径（最重要）
编辑 `env.sh`，把 `BOT_HOME` / `VENV_DIR` 改成你服务器真实路径。

- Windows 原来：`C:\Users\Administrator\Desktop\bot\...`
- Linux 常见：`$HOME/bot/...`

Python 虚拟环境：
- Windows 原来：`C:\Users\Administrator\Desktop\luna\Scripts\activate`
- Linux 对应：`$HOME/luna/bin/activate`

## 2) 前台运行（跟 Windows 双击类似）
```bash
./luna.sh
./proxy.sh
./deck.sh
./api.sh
./fliter.sh
./run.sh
```

## 3) 后台常驻（断开 SSH 也不会被杀）
每个脚本都支持 `--daemon / --stop / --status`：

```bash
./luna.sh --daemon
./luna.sh --status
tail -f "$HOME/bot_logs/lunabot.log"

./luna.sh --stop
```

其他服务同理：
```bash
./proxy.sh --daemon
./deck.sh --daemon
./api.sh --daemon
./fliter.sh --daemon
./run.sh --daemon
```

## 4) “保持终端窗口打开”的推荐方式：tmux（更像一直开着的窗口）
如果你希望“像一个永远不关闭的终端窗口”，推荐 tmux：

```bash
# 安装（按发行版选一个）
sudo apt-get update && sudo apt-get install -y tmux
# 或：sudo yum install -y tmux

tmux new -s bots
# 在 tmux 里运行：
./luna.sh
# 新开窗口：Ctrl+b 然后按 c，再运行另一个脚本
# 退出但不断开：Ctrl+b 然后按 d（detach）
# 以后再连上来：tmux attach -t bots
```

## 5) haruki.sh 说明（Haruki.exe）
Linux 服务器上最好放 Linux 可执行文件 `./Haruki`（推荐）。
如果你只有 `Haruki.exe`，脚本会尝试用 wine 运行，但不推荐长期用在服务器。

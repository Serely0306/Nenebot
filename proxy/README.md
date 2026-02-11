# OneBot WebSocket Proxy (群屏蔽)

这是一个小型的 OneBot v11 WebSocket 代理，用来在 NapCat 与你的机器人之间做中转并按群 ID 黑名单或正则规则过滤消息，目的是防止机器人在特定群里收到消息从而产生回复。

主要功能：
- 在 `client -> upstream`（NapCat -> 你的机器人）方向检测 OneBot 的消息事件，若为群消息且群号在黑名单则丢弃，不转发给上游机器人。
- 支持热加载 `blocked_groups.json`（修改后会自动生效）。
- 支持按群的正则规则（`filter_rules.json`）只过滤匹配的消息，而不拦截该群的其它消息。

使用方法：

1. 安装依赖：

```bash
cd onebot-proxy
npm install
```

2. 启动代理（默认监听 `ws://127.0.0.1:3938`，上游为 `ws://127.0.0.1:3939/onebot/v11/ws`）：

```bash
npm start
```

Python 版本（无需 npm）
--
如果你不想安装 Node/npm，我们提供了一个 Python 版本的代理：

1. 确保已安装 Python（3.8+ 推荐），并在项目目录安装依赖：

```bash
cd onebot-proxy
python -m pip install --user -r requirements.txt
```

2. 运行 Python 代理：

```bash
python python_proxy.py
```

行为与 Node 版本一致：代理监听 `ONEBOT_PROXY_PORT`（默认 3938），转发到 `ONEBOT_UPSTREAM_URL`（默认 `ws://127.0.0.1:3939/onebot/v11/ws`），支持 `blocked_groups.json` 与 `filter_rules.json` 的热加载。


可选环境变量：
- `ONEBOT_PROXY_PORT` - 代理监听端口，默认 `3938`
- `ONEBOT_UPSTREAM_URL` - 上游 OneBot 服务地址，默认 `ws://127.0.0.1:3939/onebot/v11/ws`
- `BLOCKED_FILE` - 黑名单文件路径，默认 `./blocked_groups.json`
- `FILTER_FILE` - 正则规则文件路径，默认 `./filter_rules.json`

3. 修改 NapCat 的 OneBot 配置，将原来指向上游的 URL 改为代理地址（示例文件：`resources/app/napcat/config/onebot11_548796776.json`）：

把

```json
"url": "ws://127.0.0.1:3939/onebot/v11/ws"
```

改为

```json
"url": "ws://127.0.0.1:3938/onebot/v11/ws"
```

然后重启 NapCat 或者使新的配置生效。

4. 编辑黑名单：

在 `blocked_groups.json` 中加入或移除群号（字符串或数字）。文件示例：

```json
["123456789"]
```

保存后代理会自动重新加载黑名单（无需重启代理）。

5. 使用基于正则的群内部分指令过滤

代理支持更细粒度的规则：针对特定群，只过滤匹配正则的消息内容，而不拦截该群其它消息。

- 规则文件：`filter_rules.json`，格式为对象，键为群号，值为正则字符串数组（JavaScript 风格正则，不带两边的 `/`），示例：

```json
{
	"123456789": [
		"^/ban\\b",
		"^/secret-command"
	],
	"987654321": [
		"^/private"
	]
}
```

保存后代理会热重载规则：在这些群中，只有匹配任一正则的消息会被丢弃，其他消息正常转发。

示例场景：你想在群 `123456789` 屏蔽以 `/ban` 和 `/secret-command` 开头的指令，但其它普通聊天不受影响，就把相应正则放到该群的数组中。

安全与注意事项：
- 代理不会修改其它消息内容，仅在接收到明显的 OneBot `post_type: "message"` && `message_type: "group"` 时按 `group_id` 检查并丢弃。
- 如果你的机器人原本就监听在 `127.0.0.1:3939`，请确保代理上游配置指向真实的机器人地址（不要造成端口冲突）。

如果你希望我直接替你修改 `onebot11_548796776.json` 把 `url` 指向代理（并在修改前备份原文件），回复告诉我，我可以代为修改并更新 TODO 状态。

# OneBot WebSocket Proxy (群屏蔽)

这是一个小型的 OneBot v11 WebSocket 代理，用来在 NapCat 与你的机器人之间做中转并按群 ID 黑名单过滤消息，目的是防止机器人在特定群里收到消息从而产生回复。

主要功能：
- 在 `client -> upstream`（NapCat -> 你的机器人）方向检测 OneBot 的消息事件，若为群消息且群号在黑名单则丢弃，不转发给上游机器人。
- 支持热加载 `blocked_groups.json`（修改后会自动生效）。

使用方法：

1. 安装依赖：

```bash
cd onebot-proxy
npm install
```

2. 启动代理（默认监听 `ws://127.0.0.1:3938`，上游为 `ws://127.0.0.1:3939/onebot/v11/ws`）：

```bash
npm start
```

可选环境变量：
- `ONEBOT_PROXY_PORT` - 代理监听端口，默认 `3938`
- `ONEBOT_UPSTREAM_URL` - 上游 OneBot 服务地址，默认 `ws://127.0.0.1:3939/onebot/v11/ws`
- `BLOCKED_FILE` - 黑名单文件路径，默认 `./blocked_groups.json`

3. 修改 NapCat 的 OneBot 配置，将原来指向上游的 URL 改为代理地址（示例文件：`resources/app/napcat/config/onebot11_548796776.json`）：

把

```json
"url": "ws://127.0.0.1:3939/onebot/v11/ws"
```

改为

```json
"url": "ws://127.0.0.1:3938/onebot/v11/ws"
```

然后重启 NapCat 或者使新的配置生效。

4. 编辑黑名单：

在 `blocked_groups.json` 中加入或移除群号（字符串或数字）。文件示例：

```json
["123456789"]
```

安全与注意事项：
- 代理不会修改其它消息内容，仅在接收到明显的 OneBot `post_type: "message"` && `message_type: "group"` 时按 `group_id` 检查并丢弃。
- 如果你的机器人原本就监听在 `127.0.0.1:3939`，请确保代理上游配置指向真实的机器人地址（不要造成端口冲突）。

如果你希望我直接替你修改 `onebot11_548796776.json` 把 `url` 指向代理（并在修改前备份原文件），回复告诉我，我可以代为修改并更新 TODO 状态。

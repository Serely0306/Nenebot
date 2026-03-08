# OneBotFilter 配置说明

本文档只说明 `config.yaml` 的结构、字段含义和常见写法。

## 顶层结构

```yaml
server:
  ...

bot-apps:
  - ...
```

- `server`：OneBotFilter 自身的监听、鉴权、文件服务和命令权限配置
- `bot-apps`：下游 Bot 列表，以及每个 Bot 的会话过滤和消息过滤规则

## server

示例：

```yaml
server:
  host: 0.0.0.0
  port: 3939
  suffix: /ws
  bot-id: "2230254946"
  user-agent: OneBotFilter
  default:
    private:
      mode: "on"
      ids: []
    group:
      mode: "on"
      ids: []
  buffer-size: 4096
  sleep-time: 5
  debug: false
  access-token: your-token
  file-server:
    enabled: false
    root: /root/bot
    public-url: http://example.com:3939
  command-auth:
    enabled: true
    allow-owner: true
    allow-admin: true
    super-users: []
```

字段说明：
- `host`：监听地址
- `port`：监听端口
- `suffix`：OneBot WebSocket 接入路径
- `bot-id`：预期的上游 Bot ID；配置后会校验 `X-Self-ID`
- `user-agent`：连接下游 bot-app 时使用的请求头
- `default`：默认的私聊 / 群聊会话过滤规则
- `buffer-size`：WebSocket 缓冲区大小
- `sleep-time`：下游 Bot 重连间隔，单位秒
- `debug`：是否输出调试日志
- `access-token`：上游 OneBot 接入令牌

### server.default

```yaml
default:
  private:
    mode: on
    ids: []
  group:
    mode: on
    ids: []
```

`mode` 可选值：
- `on`：全部放行
- `off`：全部阻止
- `whitelist`：仅允许 `ids` 中的会话
- `blacklist`：阻止 `ids` 中的会话

### server.file-server

```yaml
file-server:
  enabled: false
  root: /root/bot
  public-url: http://example.com:3939
```

字段说明：
- `enabled`：是否启用文件服务
- `root`：文件服务根目录
- `public-url`：对外访问地址，用于把 `file://` 转成 HTTP URL

### server.command-auth

```yaml
command-auth:
  enabled: true
  allow-owner: true
  allow-admin: true
  super-users:
    - 123456789
```

字段说明：
- `enabled`：是否启用命令权限检查
- `allow-owner`：是否允许群主执行 `/启用`、`/禁用`
- `allow-admin`：是否允许群管理员执行 `/启用`、`/禁用`
- `super-users`：超管 QQ 号列表，命中后直接允许

当前行为：
- 只检查群命令
- 权限不足时静默忽略，不回复、不转发

## bot-apps

每个下游 Bot 一个配置块。

示例：

```yaml
bot-apps:
  - name: sakura
    uri: ws://127.0.0.1:13888/onebot/v11/ws
    access-token: ""
    private:
      mode: blacklist
      ids: []
      message:
        mode: blacklist
        filters: []
        prefix: []
        prefix-replace: ""
        specific-rules: {}
    group:
      mode: blacklist
      ids: []
      message:
        mode: blacklist
        filters: []
        prefix: []
        prefix-replace: ""
        specific-rules: {}
    message:
      mode: ""
      filters: []
      prefix: []
      prefix-replace: ""
      specific-rules: {}
```

字段说明：
- `name`：Bot 名称，用于命令中的 `bot名`
- `uri`：下游 Bot 的 WebSocket 地址
- `access-token`：连接下游 Bot 时使用的令牌
- `private`：私聊会话过滤与私聊消息过滤
- `group`：群聊会话过滤与群聊消息过滤
- `message`：公共消息过滤默认值；`private.message` 和 `group.message` 可继承它

## private / group

示例：

```yaml
group:
  mode: blacklist
  ids:
    - 123456789
  message:
    mode: blacklist
    filters:
      - /
      - song
    prefix:
      - b1
    prefix-replace: /
    specific-rules: {}
```

字段说明：
- `mode`：会话级过滤模式
- `ids`：会话 ID 列表
- `message`：消息内容过滤配置

`mode` 可选值：
- `on`
- `off`
- `whitelist`
- `blacklist`
- `default`

说明：
- `private.mode` / `group.mode` 控制某个 Bot 是否接收该私聊或群聊的消息
- 当值为 `default` 或空时，会继承 `server.default.private/group.mode`

## message

示例：

```yaml
message:
  mode: blacklist
  filters:
    - /
    - song
  prefix:
    - b1
    - '#'
  prefix-replace: /
  specific-rules: {}
```

字段说明：
- `mode`：消息内容过滤模式
- `filters`：正则或普通字符串规则列表
- `prefix`：前缀直通列表
- `prefix-replace`：命中前缀后的替换内容
- `specific-rules`：对特定群号或私聊 ID 的额外规则

`message.mode` 可选值：
- `on`
- `whitelist`
- `blacklist`
- `default`

行为：
- `whitelist`：命中过滤规则才放行
- `blacklist`：命中过滤规则就拦截
- `on` 或空：直接放行
- `default`：继承公共 `message`

## prefix / prefix-replace

示例：

```yaml
prefix:
  - b1
  - '#'
prefix-replace: /
```

行为：
- 命中前缀时，把消息改写为 `prefix-replace + 去掉前缀后的内容`
- 改写后直接放行
- 未命中前缀时，继续按 `filters` 和 `mode` 处理

例如：
- `b1help` -> `/help`
- `#song` -> `/song`
- `hello` -> 不改写，继续走正常过滤

## specific-rules

用于对某个群号或私聊 ID 单独指定消息规则。

键名必须是字符串形式的数字 ID，例如：

```yaml
specific-rules:
  "985432150":
    add-filters:
      - help
    clear-prefix: true
    add-prefix:
      - '!'
    prefix-replace: ""
```

当前代码兼容两类写法。

### 1. 增量写法

可用字段：
- `mode`
- `add-filters`
- `remove-filters`
- `clear-filters`
- `add-prefix`
- `remove-prefix`
- `clear-prefix`
- `prefix-replace`

用途：
- 在父级 `message` 配置基础上增删规则
- 保留公共规则，只写差异内容

### 2. 完整写法

```yaml
specific-rules:
  "123456789":
    mode: blacklist
    filters:
      - /
      - help
    prefix:
      - '!'
    prefix-replace: ""
```

可用字段：
- `mode`
- `filters`
- `prefix`
- `prefix-replace`

用途：
- 直接为某个 ID 指定完整规则

建议：
- 已经在用增量写法时，继续保持增量写法
- 不要在同一个规则块里混合两套语义

## 命令涉及的配置

当前支持：

```text
/启用 bot名
/禁用 bot名
```

命令依赖以下配置：
- `bot-apps[].name`
- `server.command-auth`
- 对应 Bot 的 `group.ids`

行为：
- `/禁用 bot名`：把当前群加入该 Bot 的 `group.ids` 黑名单
- `/启用 bot名`：把当前群从该 Bot 的 `group.ids` 黑名单移除
- 修改后会写回 `config.yaml`
- 写回后会自动热更新

## 配置示例

```yaml
server:
  host: 0.0.0.0
  port: 3939
  suffix: /ws
  bot-id: "2230254946"
  user-agent: OneBotFilter
  default:
    private:
      mode: on
      ids: []
    group:
      mode: on
      ids: []
  buffer-size: 4096
  sleep-time: 5
  debug: false
  access-token: example-token
  file-server:
    enabled: false
    root: /root/bot
    public-url: http://127.0.0.1:3939
  command-auth:
    enabled: true
    allow-owner: true
    allow-admin: true
    super-users: []

bot-apps:
  - name: sakura
    uri: ws://127.0.0.1:13888/onebot/v11/ws
    access-token: ""
    private:
      mode: blacklist
      ids: []
      message:
        mode: blacklist
        filters:
          - /
          - song
        prefix: []
        prefix-replace: ""
        specific-rules: {}
    group:
      mode: blacklist
      ids:
        - 1234567890
      message:
        mode: blacklist
        filters:
          - /
          - song
        prefix:
          - b1
          - '#'
        prefix-replace: /
        specific-rules:
          "985432150":
            add-filters:
              - help
            clear-prefix: true
            add-prefix:
              - '!'
            prefix-replace: ""
    message:
      mode: ""
      filters: []
      prefix: []
      prefix-replace: ""
      specific-rules: {}
```

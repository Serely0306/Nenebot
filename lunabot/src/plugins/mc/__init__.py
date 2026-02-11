from .rcon import AsyncMCRcon
from ..utils import *

config = Config('mc')
logger = get_logger('MC')
file_db = get_file_db('data/mc/db.json', logger)
cd = ColdDown(file_db, logger)


QUERY_INTERVAL = config.get('query_interval')
QUEUE_CONSUME_INTERVAL = config.get('queue_consume_interval')

OFFSET_CFG = config.item('query_offset')
DISCONNECT_NOTIFY_COUNT_CFG = config.item('disconnect_notify_count')
PLAYER_TIME_UPDATE_INTERVAL_CFG = config.item('player_time_update_interval')


def timedelta2hour(td):
    return td.days * 24 + td.seconds / 3600

# MC的gametick(一天24000ticks, tick=0是早上6:00)转换为HH:MM
def gametick2time(tick):
    tick = tick % 24000
    hour = int(tick // 1000 + 6) % 24
    minute = (tick % 1000) // 100 * 6
    return f'{hour:02}:{minute:02}'


# ------------------------------------------ 服务器数据维护 ------------------------------------------ # 

# 服务端信息
class ServerData:
    def __init__(self, group_id) -> None:
        self.group_id = group_id
        
        # 从文件数据库读取配置
        self.load()

        self.first_update = True
        self.failed_count = 0
        self.failed_time = None
        self.last_failed_reason = None

        self.players = {}
        self.player_login_time = {}
        self.player_real_login_time = {}
        self.player_last_move_time = {}
        self.messages = {}

        self.next_query_ts = 0
        self.has_sucess_query = False

        self.time       = 0
        self.storming   = False
        self.thundering = False

        self.queue = []     # bot发送的消息队列

    # 保存配置
    def save(self):
        data = {
            'url': self.url,
            'listen_mode': self.listen_mode,
            'info': self.info,
            'admin': self.admin,
            'rcon_url': self.rcon_url,
            'rcon_password': self.rcon_password,
            'player_time': self.player_time,
            'game_name': self.game_name,
            'offset': self.offset,
            'chatprefix': self.chatprefix,
            'notify_on': self.notify_on,
        }
        file_db.set(f'{self.group_id}.server_info', data)
        # logger.info(f'在 {self.group_id} 中保存服务器 {data}')

    # 加载配置
    def load(self):
        data = file_db.get(f'{self.group_id}.server_info', {})
        self.url    = data.get('url', '')
        self.listen_mode = data.get('listen_mode', 'off')
        self.info   = data.get('info', '')
        self.rcon_url = data.get('rcon_url', '')
        self.rcon_password = data.get('rcon_password', '')
        self.admin = data.get('admin', [])
        self.player_time = data.get('player_time', {})
        self.game_name = data.get('game_name', 'unknown_game')
        self.offset = data.get('offset', 0)
        self.chatprefix = data.get('chatprefix', '')
        self.notify_on = data.get('notify_on', True)
        logger.info(f'在 {self.group_id} 中加载服务器 url={self.url}')


    # 向卫星地图请求
    async def query_dynamicmap(self, ts):
        url = f'{self.url}/up/world/world/{ts}'
        async with get_client_session().get(url, verify_ssl=False) as resp:
            data = await resp.text()
            json_data = loads_json(data)
            return json_data

    # 通过卫星地图发送消息
    async def send_message_by_dynamicmap(self, name, msg):
        url = f'{self.url}/up/sendmessage'
        payload = {
            'name': name,
            'message': msg
        }
        async with get_client_session().post(url, json=payload, verify_ssl=False) as resp:
            return await resp.text()

    # 通过log请求
    async def query_log(self):
        client_id = f'mybot_group_{self.group_id}'
        url = f'{self.url}/query?client_id={client_id}'
        async with get_client_session().get(url, verify_ssl=False) as resp:
            data = await resp.text()
            if resp.status != 200:
                raise Exception(f'{data}')
            json_data = loads_json(data)
            return json_data

    # 服务器执行rcon
    async def execute_rcon(self, cmd, verbose=True):
        if verbose:
            logger.info(f'发送rcon命令到{self.rcon_url}: {cmd}')
        assert_and_reply(cmd, "rcon指令不能为空")
        assert_and_reply(self.rcon_url, 'rcon地址未设置，在群聊中使用\"/setrconurl 密码\"设置')
        assert_and_reply(self.rcon_password, 'rcon密码未设置，在私聊中使用\"/setrconpw 群号 密码\"设置')

        host = self.rcon_url.split(':')[0]
        port = int(self.rcon_url.split(':')[1])
        async with AsyncMCRcon(host, self.rcon_password, port) as mcr:
            resp = await mcr.command(cmd)

        if verbose:
            logger.info(f'发送到{self.rcon_url}的rcon命令{cmd}的响应: {resp}')
        return resp
    
    # 增加玩家游玩时间
    def inc_player_time(self, account, delta):
        if self.game_name not in self.player_time:
            self.player_time[self.game_name] = {}
        if account in self.player_time[self.game_name]:
            self.player_time[self.game_name][account] += delta
        else:
            self.player_time[self.game_name][account] = delta

    # 清空当前周目玩家游玩时间
    def clear_player_time(self):
        self.player_time[self.game_name] = {}


    # 通过向服务器请求信息更新数据
    async def update(self):
        if self.listen_mode == 'off':
            return
        
        if self.listen_mode == 'log':
            data = await self.query_log()
            for item in data:
                msg_id = item['id']
                msg_ts = item['ts']
                msg_type = item['type']

                if msg_type == 'common':
                    content = item['data']['content']
                    logger.info(f'群聊 {self.group_id} 的服务器: 新消息: {item}')
                    self.queue.append(content)

                elif msg_type == 'chat':
                    player = item['data']['player']
                    if player == 'init':
                        continue
                    content = item['data']['content']
                    logger.info(f'群聊 {self.group_id} 的服务器: 新消息: {item}')
                    if content.startswith(self.chatprefix) or content.startswith('['):
                        content = content.removeprefix(self.chatprefix)
                        self.queue.append(f'<{player}> {content}')

                elif msg_type == 'join':
                    player = item['data']['player']
                    logger.info(f'群聊 {self.group_id} 的服务器: {player} 加入了游戏')
                    self.queue.append(f'{player} 加入了游戏')

                elif msg_type == 'leave':
                    player = item['data']['player']
                    logger.info(f'群聊 {self.group_id} 的服务器: {player} 离开了游戏')
                    self.queue.append(f'{player} 离开了游戏')

                elif msg_type == 'server':
                    content = item['data']['content']
                    if 'has made the advancement' in content:
                        content = content.replace('has made the advancement', '达成了进度')
                        logger.info(f'群聊 {self.group_id} 的服务器: {content}')
                        # self.queue.append(content)

        if self.listen_mode == 'dynamicmap':
            mute = self.first_update

            data = await self.query_dynamicmap(self.next_query_ts)
            current_ts = int(data['timestamp'])
            self.next_query_ts = int(current_ts + QUERY_INTERVAL * 1000 + OFFSET_CFG.get() + self.offset) 

            # 更新全局信息
            self.time       = data['servertime']
            self.storming   = data['hasStorm']
            self.thundering = data['isThundering']

            # 检测玩家上线
            for player in data['players']:
                account = player['account']
                if account not in self.players:
                    logger.info(f'群聊 {self.group_id} 的服务器: {player["name"]} 加入了游戏')
                    if not mute:
                        self.queue.append(f'{player["name"]} 加入了游戏')
                    self.players[account] = player
                    self.player_login_time[account]         = datetime.now()
                    self.player_real_login_time[account]    = datetime.now()
                    self.player_last_move_time[account]     = datetime.now()
                else:
                    # 更新玩家数据
                    if account in self.player_last_move_time:
                        if player['x'] != self.players[account]['x'] or player['y'] != self.players[account]['y'] or player['z'] != self.players[account]['z']:
                            self.player_last_move_time[account] = datetime.now()
                    self.players[account] = player

            # 检测玩家下线
            remove_list = []
            for account in self.players:
                if account not in [player['account'] for player in data['players']]:
                    logger.info(f'群聊 {self.group_id} 的服务器: {self.players[account]["name"]} 离开了游戏')
                    if not mute:
                        self.queue.append(f'{self.players[account]["name"]} 离开了游戏')
                    remove_list.append(account)

                    # 玩家下线后更新游玩时间
                    play_time = timedelta2hour(datetime.now() - self.player_login_time[account])
                    self.player_login_time.pop(account)
                    self.player_real_login_time.pop(account)
                    self.inc_player_time(account, play_time)

            # 移除下线玩家
            for account in remove_list:
                self.players.pop(account)

            # 定期更新玩家游玩时间
            player_time_updated = False
            for account in self.player_login_time:
                if datetime.now() - self.player_login_time[account] > timedelta(seconds=PLAYER_TIME_UPDATE_INTERVAL_CFG.get()):
                    self.inc_player_time(account, timedelta2hour(datetime.now() - self.player_login_time[account]))
                    self.player_login_time[account] = datetime.now()
                    player_time_updated = True
            
            # 如果有玩家下线或者游玩时间更新，保存数据
            if len(remove_list) > 0 or player_time_updated:
                self.save()

            # 检测消息更新
            for upd in data['updates']:
                logger.debug(f'群聊 {self.group_id} 的服务器: 消息更新: {upd}')
                if upd["type"] == "chat":
                    # if upd["source"] == "plugin": continue
                    key = f'{upd["timestamp"]} - {upd["account"]} - {upd["message"]}'
                    logger.info(f'群聊 {self.group_id} 的服务器: 新消息: {upd}')
                    if key not in self.messages:
                        self.messages[key] = upd
                        if not mute and (upd["message"].startswith(self.chatprefix) or upd["message"].startswith('[')):
                            msg = upd["message"].removeprefix(self.chatprefix)
                            self.queue.append(f'<{upd["playerName"]}> {msg}')
            if self.first_update:
                logger.info(f'群聊 {self.group_id} 的服务器: 通过卫星地图首次更新完成')
            self.first_update = False


    # 检查是否为管理员
    def check_admin(self, event):
        return str(event.user_id) in self.admin
    
    # 检查是否为管理员或超级用户
    def check_admin_or_superuser(self, event):
        return self.check_admin(event) or check_superuser(event)

  

# ------------------------------------------ 服务器列表维护 ------------------------------------------ #


# 服务器列表  
servers: set[ServerData] = set()

# 通过group_id获取服务器
def get_server(group_id, raise_exc=True) -> ServerData:
    for server in servers:
        if str(server.group_id) == str(group_id):
            return server
    if raise_exc:
        raise Exception(f'群 {group_id} 没有配置MC服务器')
    else:
        return None

# 通过group_id添加服务器
async def add_server(group_id):
    server = get_server(group_id, raise_exc=False)
    if server is None:
        servers.add(ServerData(group_id))
    else:
        logger.warning(f'{group_id} 的服务器已经存在')

# 通过group_id移除服务器
async def remove_server(group_id):
    server = get_server(group_id, raise_exc=False)
    if server is not None:
        servers.remove(server)
        logger.info(f'移除 {group_id} 的服务器')
    else:
        logger.warning(f'{group_id} 的服务器已经移除')

# 群白名单，同时控制服务器的开关
gwl = get_group_white_list(file_db, logger, 'mc', on_func=add_server, off_func=remove_server)

# 初始添加服务器
for group_id in gwl.get():
    servers.add(ServerData(group_id))


# ------------------------------------------ 定时任务 ------------------------------------------ #

# 向服务器请求信息
async def query_server(server: ServerData):
    if server.listen_mode != 'off':
        try:
            await server.update()
            if server.failed_count >= DISCONNECT_NOTIFY_COUNT_CFG.get():
                logger.info(f'发送重连通知到 {server.group_id}')
                if server.notify_on:
                    server.queue.append('重新建立服务器监听连接')
            server.failed_count = 0
            server.last_failed_reason = None
            server.has_sucess_query = True
        except Exception as e:
            if server.failed_count <= DISCONNECT_NOTIFY_COUNT_CFG.get():
                pass
            if server.failed_count == DISCONNECT_NOTIFY_COUNT_CFG.get():
                if server.has_sucess_query:
                    if server.notify_on:
                        server.queue.append(f'监听服务器连接断开: {e}')
                    logger.print_exc(f'{server.url} 定时查询失败达到上限: {e}，发送断连通知到 {server.group_id}')
                    server.failed_time = datetime.now()
                else:
                    logger.print_exc(f'{server.url} 定时查询失败达到上限: {e}')
            server.failed_count += 1
            server.last_failed_reason = str(e)
            server.next_query_ts = 0

# 请求所有服务器
@repeat_with_interval(QUERY_INTERVAL, '请求服务器', logger)
async def query_all_servers():
    for server in servers:
        if server.listen_mode != 'off' and server.url:
            asyncio.get_event_loop().create_task(query_server(server))

# 消费消息队列
@repeat_with_interval(QUEUE_CONSUME_INTERVAL, '消费消息队列', logger)
async def consume_queue():
    consume_queue_failed_count = 0
    for server in servers:
        try:
            while len(server.queue) > 0:
                msg = server.queue.pop(0)
                msg = f'[Server] {msg}'
                await send_group_msg_by_bot(server.group_id, msg)
                consume_queue_failed_count = 0
        except Exception as e:
            if consume_queue_failed_count < 5:
                logger.error(f'消费消息队列 {server.url} 失败: {e}')
            consume_queue_failed_count += 1


# ------------------------------------------ 聊天逻辑 ------------------------------------------ #

# 查询服务器信息
info = CmdHandler(["/info"], logger)
info.check_wblist(gwl).check_cdrate(cd).check_group()
@info.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    
    msg = f"【{server.game_name}】\n"
    msg += server.info.strip() 
    if server.info.strip() != '':
        msg += '\n------------------------\n'

    if server.listen_mode == 'off':
        msg += f"监听已关闭"
    elif server.failed_count > 0:
        msg += f"服务器监听连接断开\n"
        if server.failed_time:
            msg += f"断连时间: {server.failed_time.strftime('%Y-%m-%d %H:%M:%S')}\n"
        if server.last_failed_reason:
            msg += f"最近一次错误:\n"
            msg += server.last_failed_reason
    else:
        if server.listen_mode == 'dynamicmap':
            msg += f'服务器时间: {gametick2time(server.time)}'
            if server.thundering: msg += ' ⛈'
            elif server.storming: msg += ' 🌧'
            msg += '\n'
            msg += f'在线玩家数: {len(server.players)}\n'
            for player in server.players.values():
                msg += f'<{player["name"]}>\n'
                msg += f'{player["world"]}({player["x"]:.1f},{player["y"]:.1f},{player["z"]:.1f})\n'
                msg += f'HP:{player["health"]:.1f} Armor:{player["armor"]:.1f}\n'
                online_time = timedelta2hour(datetime.now() - server.player_real_login_time.get(player["account"], datetime.now()))
                afk_time    = timedelta2hour(datetime.now() - server.player_last_move_time.get(player["account"], datetime.now()))
                msg += f'online time: {online_time:.2f}h\n'
                if afk_time > 0.2:
                    msg += f'afk time: {afk_time:.2f}h'
        elif server.listen_mode == 'log':
            try:
                player_list = await server.execute_rcon('list')
                msg += f'{player_list}'
            except Exception as e:
                logger.print_exc(f'通过rcon查询服务器 {server.url} 玩家列表失败')
                msg += f'通过rcon获取玩家列表失败: {e}\n'
            
    return await ctx.asend_reply_msg(msg.strip())

# 更改或查询监听模式
listen = CmdHandler(["/listen"], logger)
listen.check_wblist(gwl).check_cdrate(cd).check_group()
@listen.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    pre_mode = server.listen_mode

    args = ctx.get_args().strip()
    if not args:
        return await ctx.asend_reply_msg(f'当前监听模式为 {pre_mode}')

    assert_and_reply(args in ['dynamicmap', 'log', 'off'], f'监听模式只能为 dynamicmap/log/off')
    if args == pre_mode:
        return await ctx.asend_reply_msg(f'当前监听模式已经为 {pre_mode}')
    
    server.listen_mode = args
    server.save()
    return await ctx.asend_reply_msg(f'修改监听模式： {pre_mode} -> {args}')

# 设置url
set_url = CmdHandler(["/seturl"], logger)
set_url.check_wblist(gwl).check_cdrate(cd).check_group()
@set_url.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    url = ctx.get_args().strip()
    assert_and_reply(url, '请输入正确的URL')
    if not url.startswith('http'):
        url = 'http://' + url
    server.url = url
    server.save()
    return await ctx.asend_reply_msg(f'设置MC服务器监听地址为: {url}')

# 获取url
get_url = CmdHandler(["/geturl"], logger)
get_url.check_wblist(gwl).check_cdrate(cd).check_group()
@get_url.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    return await ctx.asend_reply_msg(f'本群设置的MC服务器监听地址为: {server.url}')

# 设置info
set_info = CmdHandler(["/setinfo"], logger)
set_info.check_wblist(gwl).check_cdrate(cd).check_group()
@set_info.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    info = ctx.get_args().strip()
    server.info = info
    server.save()
    return await ctx.asend_reply_msg(f'设置MC服务器信息为: {info}')

# 发送消息
sendmsg = CmdHandler(["/send"], logger)
sendmsg.check_wblist(gwl).check_cdrate(cd).check_group()
@sendmsg.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if server.listen_mode == 'off':
        return await ctx.asend_reply_msg('MC服务器监听已关闭，无法发送消息')

    content = ctx.get_args().strip()
    user_name = ctx.get_sender_name()
    msg = f'[{user_name}] {content}'

    if server.listen_mode == 'dynamicmap':
        await server.send_message_by_dynamicmap(user_name, msg)
    if server.listen_mode == 'log':
        await server.execute_rcon(f'say {msg}')

    logger.info(f'{user_name} 发送消息到 {server.url} 成功: {msg}')

# 添加管理员
add_admin = CmdHandler(["/opadd"], logger)
add_admin.check_wblist(gwl).check_cdrate(cd).check_group().check_superuser()
@add_admin.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    cqs = extract_cq_code(ctx.get_msg())
    assert_and_reply('at' in cqs, '请@一个人')
    user_id = str(cqs['at'][0]['qq'])
    assert_and_reply(user_id not in server.admin, '该用户已经是管理员')
    server.admin.append(user_id)
    server.save()
    return await ctx.asend_reply_msg(f'添加管理员成功: {user_id}')

# 移除管理员
remove_admin = CmdHandler(["/opdel"], logger)
remove_admin.check_wblist(gwl).check_cdrate(cd).check_group().check_superuser()
@remove_admin.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    cqs = extract_cq_code(ctx.get_msg())
    assert_and_reply('at' in cqs, '请@一个人')
    user_id = str(cqs['at'][0]['qq'])
    assert_and_reply(user_id in server.admin, '该用户不是管理员')
    server.admin.remove(user_id)
    server.save()
    return await ctx.asend_reply_msg(f'移除管理员成功: {user_id}')

# 获取管理员列表
get_admin = CmdHandler(["/oplist"], logger)
get_admin.check_wblist(gwl).check_cdrate(cd).check_group()
@get_admin.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    msg = '管理员列表:\n'
    for user_id in server.admin:
        user_name = await get_group_member_name(ctx.group_id, int(user_id))
        msg += f'{user_name}({user_id})\n'
    return await ctx.asend_reply_msg(msg.strip())

# 设置rconurl
set_rcon = CmdHandler(["/setrconurl"], logger)
set_rcon.check_wblist(gwl).check_cdrate(cd).check_group()
@set_rcon.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    url = ctx.get_args().strip()
    assert_and_reply(url, '请输入正确的rcon地址')
    if not url.startswith('http'):
        url = 'http://' + url
    server.rcon_url = url
    server.save()
    return await ctx.asend_reply_msg(f'设置MC服务器rcon地址为: {url}')

# 设置rcon密码 
set_rcon_pw = CmdHandler(["/setrconpw"], logger)
set_rcon_pw.check_private().check_cdrate(cd)
@set_rcon_pw.handle()
async def _(ctx: HandlerContext):
    group_id, pw = ctx.get_args().strip().split(' ', 1)
    server = get_server(group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    server.rcon_password = pw
    server.save()
    return await ctx.asend_reply_msg(f'成功设置群组 {group_id} 的MC服务器rcon密码为: {pw}')

# 获取rconurl
get_rcon = CmdHandler(["/getrconurl"], logger)
get_rcon.check_wblist(gwl).check_cdrate(cd).check_group()
@get_rcon.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    return await ctx.asend_reply_msg(f'本群设置的MC服务器rcon地址为: {server.rcon_url}')

# 发送rcon命令
rcon = CmdHandler(["/rcon"], logger)
rcon.check_wblist(gwl).check_cdrate(cd).check_group()
@rcon.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    cmd = ctx.get_args().strip()
    resp = await server.execute_rcon(cmd, verbose=True)
    return await ctx.asend_reply_msg(f'发送成功，响应:\n{resp}' if resp else '发送成功，无响应')

# 查询游玩时间统计
playtime = CmdHandler(["/playtime", "/play_time"], logger)
playtime.check_wblist(gwl).check_cdrate(cd).check_group()
@playtime.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    msg = '游玩时间统计:\n'
    if server.game_name not in server.player_time or len(server.player_time[server.game_name]) == 0:
        msg += '暂无数据'
    else:
        for account, play_time in server.player_time[server.game_name].items():
            msg += f'{account}: {play_time:.2f}h\n'
    return await ctx.asend_reply_msg(msg.strip())

# 清空游玩时间统计
playtime_clear = CmdHandler(["/playtime_clear", "/play_time_clear"], logger)
playtime_clear.check_wblist(gwl).check_cdrate(cd).check_group()
@playtime_clear.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    server.clear_player_time()
    server.save()
    return await ctx.asend_reply_msg('成功清空游玩时间统计')

# 开始新周目
start_game = CmdHandler(["/start_game", "/startgame"], logger)
start_game.check_wblist(gwl).check_cdrate(cd).check_group()
@start_game.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    game_name = ctx.get_args().strip()
    assert_and_reply(game_name, '请输入正确的游戏名称')
    pre_name = server.game_name
    server.game_name = game_name
    server.save()
    return await ctx.asend_reply_msg(f'切换周目: {pre_name} -> {game_name}')

# 设置时间偏移
set_offset = CmdHandler(["/setoffset"], logger)
set_offset.check_wblist(gwl).check_cdrate(cd).check_group()
@set_offset.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    offset = int(ctx.get_args().strip())
    server.offset = offset
    server.save()
    return await ctx.asend_reply_msg(f'设置时间偏移为: {offset}')

# 获取时间偏移
get_offset = CmdHandler(["/getoffset"], logger)
get_offset.check_wblist(gwl).check_cdrate(cd).check_group()
@get_offset.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    return await ctx.asend_reply_msg(f'本群设置的MC服务器时间偏移为: {server.offset}')

# 设置聊天前缀
set_chatprefix = CmdHandler(["/setchatprefix"], logger)
set_chatprefix.check_wblist(gwl).check_cdrate(cd).check_group()
@set_chatprefix.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    chatprefix = ctx.get_args().strip()
    server.chatprefix = chatprefix
    server.save()
    return await ctx.asend_reply_msg(f'设置聊天前缀为: {chatprefix}')

# 获取聊天前缀
get_chatprefix = CmdHandler(["/getchatprefix"], logger)
get_chatprefix.check_wblist(gwl).check_cdrate(cd).check_group()
@get_chatprefix.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    return await ctx.asend_reply_msg(f'本群设置的MC服务器聊天前缀为: {server.chatprefix}')
    
# 开启服务器断线连线通知
notify_on = CmdHandler(["/connect_notify_on"], logger)
notify_on.check_wblist(gwl).check_cdrate(cd).check_group()
@notify_on.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    server.notify_on = True
    server.save()
    return await ctx.asend_reply_msg('开启服务器断线连线通知')

# 关闭服务器断线连线通知
notify_off = CmdHandler(["/connect_notify_off"], logger)
notify_off.check_wblist(gwl).check_cdrate(cd).check_group()
@notify_off.handle()
async def _(ctx: HandlerContext):
    server = get_server(ctx.group_id)
    if not server.check_admin_or_superuser(ctx.event): return
    server.notify_on = False
    server.save()
    return await ctx.asend_reply_msg('关闭服务器断线连线通知')
    




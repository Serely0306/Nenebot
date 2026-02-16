#!/usr/bin/env python3
"""
修改版：作为客户端连接6700端口，并转发到3939端口
行为与 Node 版本一致：在 NapCat -> 机器人 的方向按群黑名单或按群正则规则丢弃消息。

配置：
  - 环境变量 LOCAL_WS_URL 默认 ws://127.0.0.1:6700
  - 环境变量 ONEBOT_UPSTREAM_URL 默认 ws://127.0.0.1:3939/onebot/v11/ws
  - BLOCKED_FILE 默认 blocked_groups.json
  - FILTER_FILE 默认 filter_rules.json （规则为 groupId -> [regex_str,...]）

运行： python python_proxy.py
"""
import asyncio
import json
import os
import re
import time
import logging
from typing import Dict, List, Set

import websockets
from websockets import connect

logging.basicConfig(level=logging.INFO, format='[python-proxy] %(message)s')

# 修改：添加本地WebSocket地址配置
LOCAL_WS_URL = os.getenv('LOCAL_WS_URL', 'ws://127.0.0.1:6700')
UPSTREAM = os.getenv('ONEBOT_UPSTREAM_URL', 'ws://127.0.0.1:3939/ws')
UPSTREAM1 = os.getenv('ONEBOT_UPSTREAM_URL1', 'ws://127.0.0.1:3940/ws')
BLOCKED_FILE = os.getenv('BLOCKED_FILE', 'blocked_groups.json')
FILTER_FILE = os.getenv('FILTER_FILE', 'filter_rules.json')
WEBUI_CONFIG_PATH = os.getenv('WEBUI_CONFIG_PATH') or os.path.abspath(os.path.join(os.path.dirname(__file__), '..', 'NapCat.40768.Shell', 'versions', '9.9.22-40768', 'resources', 'app', 'napcat', 'config', 'webui.json'))
TOKEN = ""
_cached_blocked_mtime = 0.0
_cached_filters_mtime = 0.0
_cached_blocked: Set[str] = set()
_cached_filters: Dict[str, List[re.Pattern]] = {}

def load_blocked() -> Set[str]:
    global _cached_blocked_mtime, _cached_blocked
    try:
        mtime = os.path.getmtime(BLOCKED_FILE)
    except Exception:
        return set()
    if mtime != _cached_blocked_mtime:
        try:
            with open(BLOCKED_FILE, 'r', encoding='utf8') as f:
                arr = json.load(f)
            _cached_blocked = set(map(str, arr))
            _cached_blocked_mtime = mtime
            logging.info('blocked groups loaded: %s', list(_cached_blocked))
        except Exception as e:
            logging.error('load_blocked error: %s', e)
    return _cached_blocked

def load_filters() -> Dict[str, List[re.Pattern]]:
    global _cached_filters_mtime, _cached_filters
    try:
        mtime = os.path.getmtime(FILTER_FILE)
    except Exception:
        return {}
    if mtime != _cached_filters_mtime:
        try:
            with open(FILTER_FILE, 'r', encoding='utf8') as f:
                obj = json.load(f)
            tmp: Dict[str, List[re.Pattern]] = {}
            for gid, arr in (obj.items() if isinstance(obj, dict) else []):
                if not isinstance(arr, list):
                    continue
                patterns = []
                for p in arr:
                    try:
                        patterns.append(re.compile(p))
                    except Exception as e:
                        logging.error('invalid regex for group %s: %s', gid, e)
                tmp[str(gid)] = patterns
            _cached_filters = tmp
            _cached_filters_mtime = mtime
            logging.info('filter rules loaded for groups: %s', list(_cached_filters.keys()))
        except Exception as e:
            logging.error('load_filters error: %s', e)
    return _cached_filters


def get_local_webui_token() -> str:
    """尝试从 NapCat 的 webui.json 中读取 token 字段。
    返回 token 字符串或空字符串。
    可通过环境变量 WEBUI_CONFIG_PATH 指定自定义路径。
    """
    try:
        if not os.path.exists(WEBUI_CONFIG_PATH):
            return TOKEN
        with open(WEBUI_CONFIG_PATH, 'r', encoding='utf8') as f:
            data = json.load(f)
        token = data.get('token') if isinstance(data, dict) else ''
        if isinstance(token, str) and token:
            return token
    except Exception as e:
        logging.debug('read webui config error: %s', e)
    return ''


def get_webui_origin() -> str:
    try:
        if not os.path.exists(WEBUI_CONFIG_PATH):
            return 'http://127.0.0.1:3939'
        with open(WEBUI_CONFIG_PATH, 'r', encoding='utf8') as f:
            data = json.load(f)
        host = data.get('host', '127.0.0.1')
        port = data.get('port', 3939)
        return f"http://{host}:{port}"
    except Exception:
        return 'http://127.0.0.1:3939'


async def open_connection(uri: str, origin_hint: str = None, is_local: bool = False):
    """修改：通用的连接函数，用于连接本地6700和远程3939"""
    last_exc = None
    subprotocol_candidates = [['onebot.v11'], ['onebot-v11'], ['OneBot/11'], None]
    origins = [origin_hint, None]
    
    # 修改：如果是本地连接，从webui.json获取token
    token = get_local_webui_token() if is_local else None
    final_uri = uri
    
    if token and is_local:
        try:
            import urllib.parse
            url_parts = list(urllib.parse.urlparse(uri))
            qs = dict(urllib.parse.parse_qsl(url_parts[4]))
            qs['access_token'] = token
            url_parts[4] = urllib.parse.urlencode(qs)
            final_uri = urllib.parse.urlunparse(url_parts)
            logging.info('using token for local connection')
        except Exception as e:
            logging.error('failed to add token to URL: %s', e)
    
    for subp in subprotocol_candidates:
        for orig in origins:
            kwargs = {}
            if subp is not None:
                kwargs['subprotocols'] = subp
            if orig:
                kwargs['origin'] = orig
            kwargs['max_size'] = None
            
            # 修改：添加User-Agent头
            extra_headers = {}
            if is_local and token:
                # 也可以尝试通过header传递token
                extra_headers['Authorization'] = f'Bearer {token}'
            extra_headers['User-Agent'] = 'OneBot/11'
            kwargs['extra_headers'] = extra_headers
            
            try:
                logging.info('attempt connect -> uri=%s subprotocols=%s origin=%s', final_uri, subp, orig)
                ws = await connect(final_uri, **kwargs)
                logging.info('connect success with subprotocols=%s origin=%s', subp, orig)
                return ws
            except Exception as e:
                last_exc = e
                logging.debug('connection attempt failed: %s', e)
                await asyncio.sleep(0.1)
    
    # final fallback: try without any extras
    try:
        ws = await connect(final_uri, max_size=None)
        return ws
    except Exception as e:
        last_exc = e
    raise last_exc

def extract_text_from_event(event) -> str:
    if not event:
        return ''
    msg = event.get('message')
    if isinstance(msg, str):
        return msg
    if isinstance(msg, list):
        parts = []
        for el in msg:
            if isinstance(el, str):
                parts.append(el)
            elif isinstance(el, dict):
                text = el.get('text') or el.get('data', {}).get('text') or el.get('content', {}).get('text')
                if isinstance(text, str):
                    parts.append(text)
        return ''.join(parts)
    raw = event.get('raw_message')
    if isinstance(raw, str):
        return raw
    return ''

async def forward(src, dst, direction: str, apply_filter: bool = True):
    """修改：添加apply_filter参数，控制是否应用过滤规则"""
    try:
        async for message in src:
            drop = False
            if apply_filter:  # 只在从本地到上游的方向应用过滤
                try:
                    ev = json.loads(message)
                    if ev.get('post_type') == 'message' and ev.get('message_type') == 'group':
                        gid = ev.get('group_id') or (ev.get('data') or {}).get('group_id')
                        if gid and str(gid) in load_blocked():
                            drop = True
                            logging.info('drop group message for fully blocked group %s', gid)
                        elif gid:
                            rules = load_filters().get(str(gid))
                            if rules:
                                text = extract_text_from_event(ev)
                                for r in rules:
                                    try:
                                        if r.search(str(text)):
                                            drop = True
                                            logging.info('drop group message by rule %s %s', gid, r.pattern)
                                            break
                                    except Exception as e:
                                        logging.error('regex test error: %s', e)
                except Exception:
                    # non-json or parsing error -> forward
                    pass
            if not drop:
                try:
                    await dst.send(message)
                except Exception as e:
                    logging.error('send error: %s', e)
                    break
    except websockets.ConnectionClosed:
        logging.info('%s connection closed', direction)
    except Exception as e:
        logging.error('forward loop error: %s', e)

async def run_proxy():
    """修改：作为客户端运行，连接本地6700和远程3939"""
    logging.info('starting proxy as client...')
    logging.info('local: %s', LOCAL_WS_URL)
    logging.info('upstream: %s', UPSTREAM)
    
    local_ws = None
    upstream_ws = None
    
    while True:
        try:
            # 连接本地6700（作为客户端）
            logging.info('connecting to local: %s', LOCAL_WS_URL)
            local_ws = await open_connection(LOCAL_WS_URL, is_local=True)
            logging.info('connected to local')
            
            # 连接上游3939（作为客户端）
            logging.info('connecting to upstream: %s', UPSTREAM)
            upstream_ws = await open_connection(UPSTREAM, origin_hint=get_webui_origin(), is_local=False)
            #logging.info('connecting to upstream: %s', UPSTREAM1)
            #upstream_ws = await open_connection(UPSTREAM1, origin_hint=get_webui_origin(), is_local=False)
            logging.info('connected to upstream')
            
            # 双向转发
            # 从本地到上游：应用过滤规则
            # 从上游到本地：不应用过滤规则
            await asyncio.gather(
                forward(local_ws, upstream_ws, 'local->upstream', apply_filter=True),
                forward(upstream_ws, local_ws, 'upstream->local', apply_filter=False)
            )
            
        except Exception as e:
            logging.error('proxy error: %s', e)
            
            # 关闭连接
            if local_ws:
                try:
                    await local_ws.close()
                except:
                    pass
            if upstream_ws:
                try:
                    await upstream_ws.close()
                except:
                    pass
            
            # 等待重连
            logging.info('reconnecting in 5 seconds...')
            await asyncio.sleep(5)

def ensure_default_files():
    if not os.path.exists(BLOCKED_FILE):
        with open(BLOCKED_FILE, 'w', encoding='utf8') as f:
            json.dump([], f, ensure_ascii=False, indent=2)
    if not os.path.exists(FILTER_FILE):
        with open(FILTER_FILE, 'w', encoding='utf8') as f:
            json.dump({}, f, ensure_ascii=False, indent=2)

def main():
    """修改：不再作为服务器监听端口，而是作为客户端运行"""
    ensure_default_files()
    
    # 显示配置信息
    token = get_local_webui_token()
    if token:
        logging.info('found token from webui.json')
    else:
        logging.info('no token found in webui.json')
    
    # 运行代理
    try:
        asyncio.run(run_proxy())
    except KeyboardInterrupt:
        logging.info('proxy stopped by user')

if __name__ == '__main__':
    main()
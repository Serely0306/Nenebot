"""
MySekai 数据上传服务

用于接收用户上传的 MySekai 抓包数据（二进制或JSON），自动解密后保存到 lunabot 数据目录。
支持通过 QQ 号查询绑定的游戏账号，并选择要上传数据的账号。

使用方法：
    python server.py [--host HOST] [--port PORT]

参数：
    --host: 监听地址，默认 0.0.0.0（允许外部访问）
    --port: 监听端口，默认 5000
"""

import os
import json
import time
import argparse
from pathlib import Path
from flask import Flask, request, jsonify, send_from_directory, Response, stream_with_context
from flask_cors import CORS
import requests as http_requests  # 用于 Suite haruki 代理请求

# 解密相关导入
try:
    import msgpack
    from sssekai.crypto.APIManager import decrypt, SEKAI_APIMANAGER_KEYSETS
    DECRYPT_AVAILABLE = True
except ImportError:
    DECRYPT_AVAILABLE = False
    print("警告: sssekai 或 msgpack 未安装，二进制文件解密功能不可用")
    print("请运行: pip install sssekai msgpack")

app = Flask(__name__, static_folder='.', static_url_path='')
app.config['JSON_AS_ASCII'] = False
try:
    app.json.ensure_ascii = False
except AttributeError:
    pass
CORS(app)

# 支持的区服列表和对应的 sssekai 密钥名称
VALID_REGIONS = ['jp', 'cn', 'tw', 'kr', 'en']
REGION_TO_KEYSET = {
    'jp': 'jp',
    'cn': 'cn',
    'tw': 'tw',
    'kr': 'kr',
    'en': 'en'
}

REGION_NAMES = {
    'jp': '日服',
    'cn': '国服',
    'tw': '台服',
    'kr': '韩服',
    'en': '国际服'
}

# lunabot 数据目录（相对于当前脚本位置）
LUNABOT_BASE = Path(__file__).parent.parent / 'lunabot'
LUNABOT_DATA_BASE = LUNABOT_BASE / 'data' / 'sekai' / 'user_data'
PROFILE_DB_PATH = LUNABOT_BASE / 'data' / 'sekai' / 'profile' / 'db.json'


def load_profile_db() -> dict:
    """加载用户绑定数据库"""
    try:
        with open(PROFILE_DB_PATH, 'r', encoding='utf-8') as f:
            return json.load(f)
    except FileNotFoundError:
        return {"bind_list": {}, "main_bind_list": {}}
    except Exception as e:
        print(f"加载绑定数据库失败: {e}")
        return {"bind_list": {}, "main_bind_list": {}}


def get_bind_info(qq_id: str, region: str) -> dict:
    """获取指定 QQ 号在指定区服的绑定信息"""
    db = load_profile_db()
    bind_list = db.get("bind_list", {}).get(region, {})
    main_bind_list = db.get("main_bind_list", {}).get(region, {})
    
    qq_id = str(qq_id)
    
    # 获取绑定的游戏 ID 列表
    bound_ids = bind_list.get(qq_id, [])
    if isinstance(bound_ids, str):
        bound_ids = [bound_ids]
    
    # 获取主账号
    main_id = main_bind_list.get(qq_id, bound_ids[0] if bound_ids else None)
    
    return {
        "qq_id": qq_id,
        "region": region,
        "bound_ids": bound_ids,
        "main_id": main_id,
        "has_binding": len(bound_ids) > 0
    }


def decrypt_binary_data(binary_data: bytes, region: str) -> dict:
    """解密二进制游戏数据"""
    if not DECRYPT_AVAILABLE:
        raise RuntimeError("解密功能不可用，请安装 sssekai 和 msgpack")
    
    keyset_name = REGION_TO_KEYSET.get(region)
    if keyset_name not in SEKAI_APIMANAGER_KEYSETS:
        raise ValueError(f"不支持的区服密钥: {region}")
    
    keyset = SEKAI_APIMANAGER_KEYSETS[keyset_name]
    
    # 解密
    try:
        decrypted_data = decrypt(binary_data, keyset)
    except Exception as e:
        raise ValueError(f"解密失败: {str(e)}")
    
    # msgpack 反序列化
    try:
        data = msgpack.unpackb(decrypted_data, raw=False)
    except Exception as e:
        raise ValueError(f"msgpack 反序列化失败: {str(e)}")
    
    return data


def convert_to_serializable(obj):
    """将 msgpack 解析的对象转换为 JSON 可序列化格式"""
    if isinstance(obj, dict):
        return {k: convert_to_serializable(v) for k, v in obj.items()}
    elif isinstance(obj, list):
        return [convert_to_serializable(item) for item in obj]
    elif isinstance(obj, bytes):
        try:
            return obj.decode('utf-8')
        except:
            return obj.hex()
    else:
        return obj


def mask_game_id(game_id: str, keep: int = 6) -> str:
    """遮蔽游戏 ID，只保留最后几位"""
    game_id = str(game_id)
    if len(game_id) <= keep:
        return game_id
    return '*' * (len(game_id) - keep) + game_id[-keep:]


def load_and_filter_json(file_path, filter_keys: list):
    """加载 JSON 文件并按 filter_keys 过滤字段"""
    with open(file_path, 'r', encoding='utf-8') as f:
        data = json.load(f)
    if filter_keys:
        filtered = {k: v for k, v in data.items() if k in filter_keys}
        # 始终保留元信息字段，确保 LunaBot 数据来源显示正常
        for meta_key in ['upload_time', 'source', 'local_source']:
            if meta_key in data:
                filtered[meta_key] = data[meta_key]
        return filtered
    return data


@app.route('/')
def index():
    """提供 Suite 上传页面（主页）"""
    return send_from_directory('.', 'index.html')


@app.route('/mysekai')
def mysekai_page():
    """提供 MySekai 上传页面"""
    return send_from_directory('.', 'mysekai.html')


@app.route('/styles.css')
def styles():
    """提供 CSS 文件"""
    return send_from_directory('.', 'styles.css')


@app.route('/script.js')
def script():
    """提供 JavaScript 文件"""
    return send_from_directory('.', 'script.js')


@app.route('/api/query_binding', methods=['POST'])
def query_binding():
    """查询 QQ 号绑定的游戏账号"""
    data = request.get_json()
    if not data:
        return jsonify({'error': '请提供 JSON 数据'}), 400
    
    qq_id = data.get('qq_id', '').strip()
    region = data.get('region', 'jp').lower()
    
    if not qq_id:
        return jsonify({'error': '请输入 QQ 号'}), 400
    
    if not qq_id.isdigit():
        return jsonify({'error': 'QQ 号格式不正确'}), 400
    
    if region not in VALID_REGIONS:
        return jsonify({'error': f'不支持的区服: {region}'}), 400
    
    bind_info = get_bind_info(qq_id, region)
    
    if not bind_info['has_binding']:
        return jsonify({
            'success': False,
            'error': f'该 QQ 号在{REGION_NAMES.get(region, region)}没有绑定任何游戏账号',
            'qq_id': qq_id,
            'region': region
        })
    
    # 构建账号列表，标记主账号
    accounts = []
    for i, game_id in enumerate(bind_info['bound_ids']):
        accounts.append({
            'index': i + 1,
            'game_id': game_id,
            'display_id': mask_game_id(game_id),
            'is_main': game_id == bind_info['main_id']
        })
    
    return jsonify({
        'success': True,
        'qq_id': qq_id,
        'region': region,
        'region_name': REGION_NAMES.get(region, region),
        'accounts': accounts,
        'main_id': bind_info['main_id'],
        'main_display_id': mask_game_id(bind_info['main_id']) if bind_info['main_id'] else None
    })


@app.route('/api/mysekai/<region>/<uid>', methods=['GET'])
def get_mysekai_data(region, uid):
    """供 LunaBot 调用的 MySekai 数据获取接口（支持 mode/filter）"""
    region = region.lower()
    if region not in VALID_REGIONS:
        return jsonify({'error': f'不支持的区服: {region}'}), 404
    
    mode = request.args.get('mode', 'local')
    filter_str = request.args.get('filter', '')
    filter_keys = [k for k in filter_str.split(',') if k]
    
    local_err = None
    file_path = LUNABOT_DATA_BASE / region / 'mysekai' / f'{uid}.json'
    
    # 尝试本地
    if mode in ['local', 'auto', 'latest']:
        if file_path.exists():
            try:
                data = load_and_filter_json(file_path, filter_keys)
                return jsonify(data)
            except Exception as e:
                return jsonify({'error': f'读取数据失败: {str(e)}'}), 500
        else:
            local_err = '文件不存在'
            if mode == 'local':
                return jsonify({'local_err': local_err}), 404
    
    # 目前 mysekai 暂无 haruki 远程源，仅返回本地错误
    return jsonify({'local_err': local_err or '文件不存在'}), 404


@app.route('/api/suite/<region>/<uid>', methods=['GET'])
def get_suite_data(region, uid):
    """供 LunaBot 调用的 Suite 数据获取接口（支持 mode/filter）"""
    region = region.lower()
    if region not in VALID_REGIONS:
        return jsonify({'error': f'不支持的区服: {region}'}), 404
    
    mode = request.args.get('mode', 'auto')
    # 兼容 filter= 和 key= 两种参数格式
    filter_str = request.args.get('filter', '')
    if not filter_str:
        filter_str = request.args.get('key', '')
    filter_keys = [k for k in filter_str.rstrip(',').split(',') if k]
    
    local_err = None
    haruki_err = None
    local_path = LUNABOT_DATA_BASE / region / 'suite' / f'{uid}.json'
    
    # 辅助函数：获取本地数据
    def get_local_data():
        nonlocal local_err
        if not local_path.exists():
            local_err = '文件不存在'
            return None
        try:
            return load_and_filter_json(local_path, filter_keys)
        except Exception as e:
            local_err = str(e)
            return None
    
    # 辅助函数：获取 haruki 数据
    def get_haruki_data():
        nonlocal haruki_err
        haruki_url = f"https://suite-api.haruki.seiunx.com/public/{region}/suite/{uid}"
        if filter_keys:
            haruki_url += f"?key={','.join(filter_keys)}"
        try:
            resp = http_requests.get(haruki_url, timeout=15)
            if resp.ok:
                data = resp.json()
                if data is not None:
                    return data
                haruki_err = '返回数据为空'
                return None
            haruki_err = f"HTTP {resp.status_code}"
            return None
        except Exception as e:
            haruki_err = str(e)
            return None
    
    # local 模式：仅本地
    if mode == 'local':
        data = get_local_data()
        if data:
            return jsonify(data)
        return jsonify({'local_err': local_err}), 404
    
    # haruki 模式：仅 haruki
    if mode == 'haruki':
        data = get_haruki_data()
        if data:
            return jsonify(data)
        return jsonify({'haruki_err': haruki_err}), 404
    
    # latest 模式：两边都取，比较 upload_time 返回最新的
    if mode == 'latest':
        local_data = get_local_data()
        haruki_data = get_haruki_data()
        
        if local_data and haruki_data:
            local_time = local_data.get('upload_time', 0)
            haruki_time = haruki_data.get('upload_time', 0)
            
            # Haruki API 返回的是秒级时间戳，如果小于 100 亿则认为是秒，转为毫秒
            if haruki_time < 10000000000:
                haruki_time *= 1000
                
            if local_time >= haruki_time:
                return jsonify(local_data)
            else:
                return jsonify(haruki_data)
        elif local_data:
            return jsonify(local_data)
        elif haruki_data:
            return jsonify(haruki_data)
        return jsonify({'local_err': local_err, 'haruki_err': haruki_err}), 404
    
    # auto 模式（默认）：本地优先，失败回退 haruki
    local_data = get_local_data()
    if local_data:
        return jsonify(local_data)
    
    haruki_data = get_haruki_data()
    if haruki_data:
        return jsonify(haruki_data)
    
    return jsonify({'local_err': local_err, 'haruki_err': haruki_err}), 404


# ==================== 订阅相关 API ====================

@app.route('/api/mysekai/<region>/upload_times', methods=['GET','POST'])
def get_mysekai_upload_times(region):
    """批量获取用户 mysekai 数据的上传时间（供 LunaBot MSR 订阅推送使用）"""
    region = region.lower()
    if region not in VALID_REGIONS:
        return jsonify({'error': f'不支持的区服: {region}'}), 404
    
    uid_modes = request.get_json()  # [(uid, mode), ...]
    if not uid_modes:
        return jsonify([])  
    
    upload_times = []
    for uid, mode in uid_modes:
        file_path = LUNABOT_DATA_BASE / region / 'mysekai' / f'{uid}.json'
        if file_path.exists():
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    data = json.load(f)
                upload_times.append(data.get('upload_time', 0))
            except Exception:
                upload_times.append(0)
        else:
            upload_times.append(0)
    
    return jsonify(upload_times)


@app.route('/api/mysekai/<region>/msr_sub', methods=['PUT'])
def update_msr_sub(region):
    """更新 MSR 订阅用户列表（供 LunaBot 定时任务使用）"""
    region = region.lower()
    uid_modes = request.get_json()
    print(f"更新 {region} MSR 订阅: {len(uid_modes or [])} 个用户")
    return jsonify({'success': True, 'count': len(uid_modes or [])})


# ==================== 代理抓包相关 ====================

import requests

# 官方 API 域名映射
REGION_API_HOSTS = {
    'jp': 'production-game-api.sekai.colorfulpalette.org',
    'cn': 'mkcn-prod-public-60001-2.dailygn.com',  # 注意：国服可能有多个域名，通常 ending 为 1-1 或 1-2
    'tw': 'prod-api.sekai-pl.com',
    'kr': 'prod-api.sekai-m.com',
    'en': 'production-game-api.sekai.colorfulstage.com'
}

def process_and_save_data(region, uid, data_bytes):
    """处理并保存抓取的数据（解密、注入ID、保存）"""
    try:
        # 1. 解密
        if not DECRYPT_AVAILABLE:
            print("警告: 解密依赖未安装，无法保存代理抓取的数据")
            return
        
        try:
            decrypted_data = decrypt_binary_data(data_bytes, region)
            data = convert_to_serializable(decrypted_data)
        except Exception as e:
            print(f"代理数据解密失败: {e}")
            return

        # 2. 注入必要字段
        if 'upload_time' not in data:
            data['upload_time'] = int(time.time() * 1000)
        
        data['source'] = 'proxy_upload'
        data['local_source'] = 'proxy_upload'

        # 注入用户 ID (如果数据中没有)
        if 'updatedResources' in data:
            if 'userMysekaiGamedata' not in data['updatedResources']:
                data['updatedResources']['userMysekaiGamedata'] = {}
            # 始终确保 userId 存在且正确
            data['updatedResources']['userMysekaiGamedata']['userId'] = int(uid)

        # 3. 保存
        save_dir = LUNABOT_DATA_BASE / region / 'mysekai'
        save_dir.mkdir(parents=True, exist_ok=True)
        save_path = save_dir / f'{uid}.json'
        
        with open(save_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
            
        print(f"代理抓包成功: {region} user {uid} -> {save_path}")
        
    except Exception as e:
        print(f"处理代理数据时出错: {e}")


@app.route('/api/<region>/user/<uid>/upload/mysekai', methods=['GET', 'POST', 'PUT'])
def proxy_upload(region, uid):
    """
    HTTP 代理接口：接收游戏客户端的请求，转发给官方服务器，并捕获响应数据
    也支持直接 POST 已解密的 JSON 数据 (用于 SekaiCatcher 等工具)
    """
    region = region.lower()
    if region not in REGION_API_HOSTS:
        return jsonify({'error': f'Unsupported region: {region}'}), 400

    # 检查是否是直接上传 JSON 数据 (Content-Type: application/json)
    content_type = request.headers.get('Content-Type', '')
    if 'application/json' in content_type:
        # 直接处理 JSON 数据（来自 SekaiCatcher 等工具）
        try:
            data = request.get_json()
            if not data:
                return jsonify({'error': '无效的 JSON 数据'}), 400
            
            # 注入必要字段
            if 'upload_time' not in data:
                data['upload_time'] = int(time.time() * 1000)
            
            if 'source' not in data:
                data['source'] = 'direct_upload'
            if 'local_source' not in data:
                data['local_source'] = 'direct_upload'
            
            # 确保 userId 存在
            if 'updatedResources' in data:
                if 'userMysekaiGamedata' not in data['updatedResources']:
                    data['updatedResources']['userMysekaiGamedata'] = {}
                data['updatedResources']['userMysekaiGamedata']['userId'] = int(uid)
            
            # 保存文件
            save_dir = LUNABOT_DATA_BASE / region / 'mysekai'
            save_dir.mkdir(parents=True, exist_ok=True)
            save_path = save_dir / f'{uid}.json'
            
            with open(save_path, 'w', encoding='utf-8') as f:
                json.dump(data, f, ensure_ascii=False, indent=2)
            
            print(f"直接上传成功: {region} user {uid} -> {save_path}")
            
            return jsonify({
                'success': True,
                'message': f'{region} user {uid} data saved',
                'file_path': str(save_path)
            })
            
        except Exception as e:
            print(f"处理直接上传数据时出错: {e}")
            return jsonify({'error': str(e)}), 500

    # 否则执行代理转发逻辑
    target_host = REGION_API_HOSTS[region]
    # 构建完整目标 URL (保持客户端请求的路径结构)
    # 注意：客户端通常请求的是 /api/user/{uid}/mysekai?xxx
    # 我们的路由也是 /api/<region>/user/<uid>/upload/mysekai
    # 但真实服务器路径通常是 /api/user/{uid}/mysekai
    
    # 从请求路径中提取真实 API 路径
    # 假设我们只代理 /mysekai 接口，所以可以直接构造
    real_api_path = f"/api/user/{uid}/mysekai"
    
    target_url = f"https://{target_host}{real_api_path}"
    
    # 转发请求 Headers (过滤掉一些可能引起问题的 headers)
    excluded_headers = ['Host', 'Content-Length']
    headers = {k: v for k, v in request.headers if k not in excluded_headers}
    headers['Host'] = target_host
    
    try:
        # 发送转发请求
        resp = requests.request(
            method=request.method,
            url=target_url,
            headers=headers,
            data=request.get_data(),
            cookies=request.cookies,
            params=request.args,
            timeout=10,
            allow_redirects=False
        )
        
        # 如果请求成功 (200 OK)，且有内容，尝试异步处理保存
        # MySekai 接口通常返回 200，内容是二进制 msgpack
        if resp.status_code == 200 and resp.content:
            # 异步处理（这里简单起见同步调用，以免多线程复杂化，
            # 如果文件很大影响延迟，可以考虑 threading.Thread）
            process_and_save_data(region, uid, resp.content)
            
        # 构造返回给客户端的响应
        response = app.make_response(resp.content)
        response.status_code = resp.status_code
        
        # 转发响应 Headers
        for k, v in resp.headers.items():
            if k not in ['Transfer-Encoding', 'Content-Encoding', 'Content-Length']:
                response.headers[k] = v
                
        return response

    except Exception as e:
        print(f"代理请求失败: {e}")
        return jsonify({'error': 'Proxy error'}), 502


# ==================== Suite 帮助接口 ====================

@app.route('/suite/help/ios', methods=['GET'])
def suite_help_ios():
    """生成 Suite iOS (Surge/Loon) 模块配置"""
    host = request.headers.get('Host')
    
    template = f"""#!name=Suite Upload Helper
#!desc=自动抓取 Suite 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=2
#!mitm=2
#!total=4
#按需修改配置注释

[Script]
# 国服
SCRIPT_cn_suite_1 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js
SCRIPT_cn_suite_2 = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js

# 日服 (取消注释以启用)
# SCRIPT_jp_suite = type=http-response, requires-body=1, binary-body-mode=1, max-size=100000000, timeout=60, pattern=^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/suite\\/user\\/(\\d+)$, script-path=http://{host}/public/scripts/upload.js

[MITM]
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com

"""
    return template, 200, {'Content-Type': 'text/plain; charset=utf-8'}


# ==================== MySekai 帮助接口 ====================

@app.route('/mysekai/help/ios', methods=['GET'])
def mysekai_help_ios():
    """生成 MySekai iOS (Surge/Loon) 模块配置"""
    host = request.headers.get('Host')
    
    template = f"""#!name=MySekai Upload Helper
#!desc=自动抓取 MySekai 数据并上传到 LunaBot 服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=3
#!mitm=3
#!total=6
#按需修改配置注释

[URL Rewrite]
# 日服 (取消注释以启用)
# ^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/jp/user/$1/upload/mysekai 307
# ^https:\\/\\/submit\\.backtrace\\.io\\/  reject

# 国服
^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/submit\\.backtrace\\.io\\/  reject

[MITM]
# hostname=%APPEND% production-game-api.sekai.colorfulpalette.org, submit.backtrace.io
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com, submit.backtrace.io

"""
    return template, 200, {'Content-Type': 'text/plain; charset=utf-8'}


@app.route('/mysekai/help/android', methods=['GET'])
def mysekai_help_android():
    """生成 MySekai Android 抓包配置指南"""
    host = request.headers.get('Host')
    
    info = f"""# MySekai Android 抓包上传配置指南

Android 使用与 haruki-proxy 类似的原理，具体方法参考 haruki-proxy 教程
将脚本替换成我所提供的内容
脚本地址在/upload后替换为 /module/android/scripts
## 与harukiproxy不同之处
1. 如果没有harukiproxy证书，注释掉config中指向harukiproxy证书的两行，安装catcher证书后需要重启虚拟机，可在完成2后一起重启
2. 需要手动设置代理，在设置中的Wifi处长按当前wifi，设置手动代理为127.0.0.1:8888，修改完后重启虚拟机
## 注意事项
1. CA证书优先使用 haruki-proxy 所安装的证书
2. 关闭进程请使用下面的脚本，仅关闭终端无法关闭进程

## 停止脚本
```bash
#!/bin/sh
# 停止 Catcher 并清理代理设置

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

echo "正在停止 Catcher..."

# 停止进程
pkill -f "Catcher-android-arm64" 2>/dev/null
pkill -f "catcher" 2>/dev/null

# 清除代理设置
settings put global http_proxy :0 2>/dev/null
settings put global https_proxy :0 2>/dev/null

echo "完成！已停止进程并清除代理设置"
```
"""
    return info, 200, {'Content-Type': 'text/plain; charset=utf-8'}


# ==================== 旧版模块接口 (保留兼容) ====================

@app.route('/module/ios', methods=['GET'])
def generate_ios_module():
    """生成 iOS (Surge/Loon) 模块配置 - 旧版兼容"""
    host = request.headers.get('Host')
    
    template = f"""#!name=MySekai Upload Helper
#!desc=自动抓取 MySekai 数据并上传到 LunaBot 本地服务器 ({host})
#!author=Nene-LunaBot
#!system=ios
#!redirect=3
#!mitm=3
#!total=6
#按需修改配置注释
[URL Rewrite]
# 日服
# ^https:\\/\\/production-game-api\\.sekai\\.colorfulpalette\\.org\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/jp/user/$1/upload/mysekai 307
# ^https:\\/\\/submit\\.backtrace\\.io\\/  reject
# 国服
^https:\\/\\/mkcn-prod-public-60001-1\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/mkcn-prod-public-60001-2\\.dailygn\\.com\\/api\\/user\\/(\\d+)\\/mysekai\\?isForceAllReloadOnlyMysekai\\=(True|False)$ http://{host}/api/cn/user/$1/upload/mysekai 307
^https:\\/\\/submit\\.backtrace\\.io\\/  reject

[MITM]
# hostname=%APPEND% production-game-api.sekai.colorfulpalette.org, submit.backtrace.io
hostname=%APPEND% mkcn-prod-public-60001-1.dailygn.com, mkcn-prod-public-60001-2.dailygn.com, submit.backtrace.io

"""
    return template, 200, {'Content-Type': 'text/plain; charset=utf-8'}

@app.route('/module/android', methods=['GET'])
def generate_android_config():
    """生成 Android (HttpCanary/Postern) 参考配置"""
    host = request.headers.get('Host')
    
    info = f"""# Android 抓包上传配置指南

Android 使用与 haruki-proxy 类似的原理，具体方法参考 haruki-proxy 教程
将脚本替换成我所提供的内容
脚本地址在当前网址后加上 /scripts
## 与harukiproxy不同之处
1. 如果没有harukiproxy证书，注释掉config中指向harukiproxy证书的两行，安装catcher证书后需要重启虚拟机，可在完成2后一起重启
2. 需要手动设置代理，在设置中的Wifi处长按当前wifi，设置手动代理为127.0.0.1:8888，修改完后重启虚拟机
## 注意事项
1. CA证书优先使用 haruki-proxy 所安装的证书
2. 关闭进程请使用下面的脚本，仅关闭终端无法关闭进程

#!/bin/sh
# 停止 Catcher 并清理代理设置

# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi

echo "正在停止 Catcher..."

# 停止进程
pkill -f "Catcher-android-arm64" 2>/dev/null
pkill -f "catcher" 2>/dev/null

# 清除代理设置
settings put global http_proxy :0 2>/dev/null
settings put global https_proxy :0 2>/dev/null

echo "完成！已停止进程并清除代理设置"
"""
    return info, 200, {'Content-Type': 'text/plain; charset=utf-8'}

@app.route('/module/android/scripts', methods=['GET'])
def generate_android_scripts_config():
    """
    生成 Android 自动部署脚本 (流式响应)
    适用于 Android 7 / 虚拟机环境
    """
    host = request.headers.get('Host')
    
    def generate():
        # --- 脚本头部 ---
        yield "#!/bin/sh\n"
        yield "# ============================================================\n"
        yield "# Catcher 自动部署脚本\n"
        yield "# 用于在 Android 设备/模拟器上自动下载并运行抓包工具\n"
        yield "# \n"
        yield "# ============================================================\n\n"

        # --- 获取 Root 权限 ---
        yield """# 获取 Root 权限
if [ "$(id -u)" -ne 0 ]; then
    if command -v su >/dev/null 2>&1; then
        exec su -c "$0 $*"
    elif command -v sudo >/dev/null 2>&1; then
        exec sudo "$0" "$@"
    else
        echo "此脚本需要 Root 权限运行" >&2
        exit 1
    fi
fi\n\n"""

        # --- 配置区域 ---
        yield "# ============================================================\n"
        yield "# 配置区域 (根据需要修改)\n"
        yield "# ============================================================\n\n"
        
        yield """# 工作目录
ROOT_DIR="/data/local/tmp/catcher"

# 可执行文件名和下载地址
BIN_NAME="Catcher-android-arm64"
"""
        yield f'BIN_URL="http://{host}/mysekai_upload/download/$BIN_NAME"\n\n'
        
        yield """# 配置文件名和下载地址
CONFIG_NAME="config-android.yaml"
"""
        yield f'CONFIG_URL="http://{host}/mysekai_upload/download/$CONFIG_NAME"\n\n'

        yield """# 默认留空自动生成新证书
EXTERNAL_CERT_PATH=""
EXTERNAL_KEY_PATH=""

# 外部证书路径 (如果使用过HarukiProxy服务则去掉注释使用 HarukiProxy 的证书)
# EXTERNAL_CERT_PATH="/data/local/tmp/harukiproxy/ca.pem"
# EXTERNAL_KEY_PATH="/data/local/tmp/harukiproxy/ca.key"

# ============================================================
# 脚本逻辑
# ============================================================

echo "=================================================="
echo "  Catcher 自动部署脚本"
echo "=================================================="

# 确保工作目录存在
if [ ! -d "$ROOT_DIR" ]; then
    echo "[1/4] 创建工作目录: $ROOT_DIR"
    mkdir -p "$ROOT_DIR"
fi

cd "$ROOT_DIR"

# 下载可执行文件 (如果不存在)
if [ ! -f "$BIN_NAME" ]; then
    echo "[2/4] 下载可执行文件..."
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$BIN_NAME" "$BIN_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$BIN_NAME" "$BIN_URL"
    else
        echo "错误: 未找到 curl 或 wget，无法下载文件"
        echo "请手动将 $BIN_NAME 推送到 $ROOT_DIR/"
        exit 1
    fi
    chmod 0755 "$BIN_NAME"
else
    echo "[2/4] 可执行文件已存在，跳过下载"
fi

# 下载配置文件 (如果不存在)
if [ ! -f "$CONFIG_NAME" ]; then
    echo "[3/4] 下载配置文件..."
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$CONFIG_NAME" "$CONFIG_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$CONFIG_NAME" "$CONFIG_URL"
    else
        echo "警告: 无法下载配置文件，将使用默认配置"
    fi
else
    echo "[3/4] 配置文件已存在，跳过下载"
fi

# 设置 DNS (某些虚拟机需要)
echo "[4/4] 配置网络环境..."
if [ ! -f /etc/resolv.conf ]; then
    mount -o remount,rw /system 2>/dev/null
    touch /etc/resolv.conf 2>/dev/null
fi
if ! grep -q "223.5.5.5" /etc/resolv.conf 2>/dev/null; then
    mount -o remount,rw /system 2>/dev/null
    echo "nameserver 223.5.5.5" >> /etc/resolv.conf 2>/dev/null
    echo "nameserver 223.6.6.6" >> /etc/resolv.conf 2>/dev/null
fi

echo ""
echo "=================================================="
echo "  准备完成，正在启动 Catcher..."
echo "=================================================="
echo ""

# 运行程序
exec ./"$BIN_NAME" -config "$CONFIG_NAME"
"""

    # 返回流式响应，Content-Type 设为 shell script
    return Response(stream_with_context(generate()), mimetype='text/x-shellscript')

@app.route('/download/<filename>')
def download_file(filename):
    return send_from_directory('/root/bot/catcher/', filename)

@app.route('/upload/<data_type>', methods=['POST'])
def upload(data_type):
    """处理文件上传（支持 suite/mysekai 等类型）"""
    # 检查是否有文件
    if 'file' not in request.files:
        return jsonify({'error': '没有上传文件'}), 400
    
    file = request.files['file']
    if file.filename == '':
        return jsonify({'error': '没有选择文件'}), 400
    
    # 获取参数
    region = request.form.get('region', 'jp').lower()
    game_id = request.form.get('game_id', '').strip()
    
    if region not in VALID_REGIONS:
        return jsonify({'error': f'不支持的区服: {region}'}), 400
    
    if not game_id:
        return jsonify({'error': '请选择要上传数据的游戏账号'}), 400
    
    if not game_id.isdigit():
        return jsonify({'error': '游戏 ID 格式不正确'}), 400
    
    try:
        # 读取文件内容
        content = file.read()
        
        # 检测文件类型并解析
        data = None
        is_binary = False
        
        # 尝试作为 JSON 解析
        try:
            text_content = content.decode('utf-8')
            data = json.loads(text_content)
        except (UnicodeDecodeError, json.JSONDecodeError):
            # 不是 JSON，尝试作为二进制解密
            is_binary = True
        
        # 如果是二进制文件，进行解密
        if is_binary:
            if not DECRYPT_AVAILABLE:
                return jsonify({
                    'error': '服务器未安装解密依赖，无法处理二进制文件。请安装 sssekai 和 msgpack'
                }), 500
            
            try:
                data = decrypt_binary_data(content, region)
                data = convert_to_serializable(data)  # 转换为 JSON 可序列化格式
            except Exception as e:
                return jsonify({'error': f'二进制文件解密失败: {str(e)}'}), 400
        
        # 添加 upload_time 字段（如果不存在）
        if 'upload_time' not in data:
            data['upload_time'] = int(time.time() * 1000)  # 毫秒级时间戳
        
        # 添加数据来源标记
        data['source'] = 'local_upload'
        data['local_source'] = 'web_upload'
        
        # 在数据中添加 userId 字段（兼容 lunabot 的处理逻辑，仅适用于 mysekai）
        if data_type == 'mysekai' and 'updatedResources' in data:
            if 'userMysekaiGamedata' not in data['updatedResources']:
                data['updatedResources']['userMysekaiGamedata'] = {}
            data['updatedResources']['userMysekaiGamedata']['userId'] = int(game_id)
        
        # 创建保存目录（根据 data_type 创建子目录）
        save_dir = LUNABOT_DATA_BASE / region / data_type
        save_dir.mkdir(parents=True, exist_ok=True)
        
        # 以游戏 ID 命名保存文件
        save_path = save_dir / f'{game_id}.json'
        with open(save_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        
        print(f"表单上传成功: {region} user {game_id} ({data_type}) -> {save_path}")
        
        return jsonify({
            'success': True,
            'user_id': game_id,
            'display_id': mask_game_id(game_id),
            'region': region,
            'region_name': REGION_NAMES.get(region, region),
            'data_type': data_type,
            'file_path': str(save_path),
            'upload_time': data['upload_time'],
            'was_binary': is_binary
        })
        
    except json.JSONDecodeError as e:
        return jsonify({'error': f'JSON 解析错误: {str(e)}'}), 400
    except Exception as e:
        return jsonify({'error': f'处理文件时出错: {str(e)}'}), 500


@app.route('/status', methods=['GET'])
def status():
    """服务状态检查"""
    return jsonify({
        'status': 'ok',
        'version': '2.0.0',
        'supported_regions': VALID_REGIONS,
        'decrypt_available': DECRYPT_AVAILABLE,
        'profile_db_available': PROFILE_DB_PATH.exists()
    })


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='MySekai 数据上传服务')
    parser.add_argument('--host', default='0.0.0.0', help='监听地址 (默认: 0.0.0.0，允许外部访问)')
    parser.add_argument('--port', type=int, default=5000, help='监听端口 (默认: 5000)')
    parser.add_argument('--debug', action='store_true', help='开启调试模式')
    args = parser.parse_args()
    
    decrypt_status = "✓ 已启用" if DECRYPT_AVAILABLE else "✗ 未安装依赖"
    db_status = "✓ 已连接" if PROFILE_DB_PATH.exists() else "✗ 未找到"
    
    print(f"""
╔════════════════════════════════════════════════════════════════╗
║             MySekai 数据上传服务 v2.0 - LunaBot                 ║
╠════════════════════════════════════════════════════════════════╣
║  本机访问: http://127.0.0.1:{args.port:<5}                              ║
║  局域网访问: http://<本机IP>:{args.port:<5}                            ║
║  二进制解密: {decrypt_status:<20}                            ║
║  绑定数据库: {db_status:<20}                            ║
╠════════════════════════════════════════════════════════════════╣
║  新功能: 输入 QQ 号查询绑定账号，选择后上传数据                 ║
╚════════════════════════════════════════════════════════════════╝
    """)
    
    app.run(host=args.host, port=args.port, debug=args.debug)

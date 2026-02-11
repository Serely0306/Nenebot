from .utils import *
from aiohttp import ClientSession, ClientConnectionError, ClientTimeout

REQUEST_TIMEOUT = 5.0

_session: ClientSession | None = None

def get_session() -> ClientSession:
    global _session
    if _session is None or _session.closed:
        _session = ClientSession(timeout=ClientTimeout(total=REQUEST_TIMEOUT))
    return _session

async def close_session():
    global _session
    if _session is not None and not _session.closed:
        await _session.close()


gameapi_config = Config('sekai.gameapi')

@dataclass
class GameApiConfig:
    api_status_url: Optional[str] = None
    profile_api_url: Optional[str] = None 
    suite_api_url: Optional[str] = None
    mysekai_api_url: Optional[str] = None  
    mysekai_photo_api_url: Optional[str] = None 
    mysekai_upload_time_api_url: Optional[str] = None 
    update_msr_sub_api_url: Optional[str] = None
    ranking_api_url: Optional[str] = None
    ranking_border_api_url: Optional[str] = None
    ranking_top100_api_url: Optional[str] = None
    ranking_near_user_api_url: Optional[str] = None
    ranking_user_myself_api_url: Optional[str] = None
    send_boost_api_url: Optional[str] = None
    create_account_api_url: Optional[str] = None
    ad_result_update_time_api_url: Optional[str] = None
    ad_result_api_url: Optional[str] = None


# 获取游戏api相关配置
def get_gameapi_config(region: str) -> GameApiConfig:
    return GameApiConfig(**(gameapi_config.get(region, {})))


# 请求游戏API data_type: json/bytes/None
import asyncio  # 必须导入

# 请求游戏API data_type: json/bytes/None
async def request_gameapi(url: str, method: str = 'GET', data_type: str | None = 'json', fetch_all_rankings: bool = True, **kwargs):
    # --- 修改开始: 并发获取逻辑 ---
    # 如果开启了合并获取模式，且当前URL包含 ranking-border
    if fetch_all_rankings and "ranking-border" in url:
        # 1. 构造另一个接口的 URL (将 border 替换为 top100)
        url_top100 = url.replace("ranking-border", "ranking-top100")
        
        # 2. 创建两个任务
        # 注意：递归调用时必须把 fetch_all_rankings 设为 False，否则会死循环
        task_border = request_gameapi(url, method, data_type, fetch_all_rankings=False, **kwargs)
        task_top100 = request_gameapi(url_top100, method, data_type, fetch_all_rankings=False, **kwargs)
        
        try:
            # 3. 并发执行并等待结果
            border_data, top100_data = await asyncio.gather(task_border, task_top100)
            
            # 4. 返回合并数据
            return {
                "border": border_data,
                "top100": top100_data
            }
        except Exception as e:
            # 如果任一请求失败，根据你的业务逻辑决定是抛出异常还是返回部分数据
            # 这里选择直接抛出，由上层处理
            raise e
    # --- 修改结束 ---

    debug(f"请求游戏API后端: {method} {url}")
    token = config.get('gameapi_token', '')
    headers = { 'Authorization': f'Bearer {token}' }
    try:
        async with get_session().request(method, url, headers=headers, verify_ssl=False, **kwargs) as resp:
            if resp.status != 200:
                try:
                    detail = await resp.text()
                    detail = loads_json(detail)['detail']
                except:
                    detail = "Unknown error" # 防止 detail 未定义
                    pass
                error(f"请求游戏API后端 {url} 失败: {resp.status} {detail}")
                raise Exception(f"请求游戏API后端失败: {resp.status} {detail}")
            
            if data_type is None:
                return resp
            elif data_type == 'json':
                if "text/plain" in resp.content_type:
                    return loads_json(await resp.text())
                elif "application/octet-stream" in resp.content_type:
                    import io
                    return loads_json(io.BytesIO(await resp.read()).read())
                else:
                    return await resp.json()
            elif data_type == 'bytes':
                return await resp.read()
            else:
                raise Exception(f"不支持的数据类型: {data_type}")
                
    except ClientConnectionError as e:
        raise Exception(f"连接游戏API后端失败")



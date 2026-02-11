from ...utils import *
from ...record import before_record_hook
from ..common import *
from ..handler import *
from ..asset import *
from ..draw import *
from .music import (
    search_music, 
    MusicSearchOptions, 
    MusicSearchResult, 
    extract_diff, 
    get_music_diff_info,
)
from .chart import generate_music_chart
from .card import (
    get_card_image, 
    has_after_training, 
    only_has_after_training, 
    get_character_name_by_id, 
    get_unit_by_card_id,
)
from .gacha import (
    spin_gacha,
    parse_search_gacha_args,
    compose_gacha_spin_image,
    SINGLE_GACHA_HELP,
)
from PIL.Image import Transpose
from PIL import ImageOps
import pydub

DEFAULT_ENTERTAINMENT_DAILY_LIMIT = 200

# ======================= 用户娱乐功能限制配置 ======================= #

# 猜x类功能限制（猜曲封、猜谱面、猜卡面、听歌识曲）
DEFAULT_GUESS_USER_COOLDOWN = 10 * 60      # 10分钟冷却（秒）
DEFAULT_GUESS_USER_DAILY_LIMIT = 30        # 每人每日30次

# 抽卡功能限制（独立配置）
DEFAULT_GACHA_USER_COOLDOWN = 5 * 60       # 5分钟冷却（秒）
DEFAULT_GACHA_USER_DAILY_LIMIT = 50        # 每人每日50次


# ======================= 用户限制核心函数 ======================= #

async def check_group_admin_or_super(ctx: SekaiHandlerContext) -> bool:
    """检查是否为群管理员或超管"""
    if check_superuser(ctx.event):
        return True
    if not ctx.group_id:
        return False
    try:
        info = await ctx.bot.call_api(
            'get_group_member_info', 
            group_id=ctx.group_id, 
            user_id=ctx.user_id
        )
        return info.get('role') in ['admin', 'owner']
    except Exception as e:
        logger.warning(f"获取群成员信息失败: {e}")
        return False


def get_user_limit_config(group_id: int, limit_type: str) -> dict:
    """
    获取用户限制配置
    limit_type: 'guess' 或 'gacha'
    返回 {'cooldown': 秒数, 'daily_limit': 次数}
    """
    group_id_str = str(group_id) if group_id else "global"
    
    # 全局默认配置
    global_config = file_db.get("entertainment_user_limit_config", {}).get("global", {})
    if limit_type == 'guess':
        default_cooldown = global_config.get('guess_cooldown', DEFAULT_GUESS_USER_COOLDOWN)
        default_daily = global_config.get('guess_daily_limit', DEFAULT_GUESS_USER_DAILY_LIMIT)
    else:  # gacha
        default_cooldown = global_config.get('gacha_cooldown', DEFAULT_GACHA_USER_COOLDOWN)
        default_daily = global_config.get('gacha_daily_limit', DEFAULT_GACHA_USER_DAILY_LIMIT)
    
    # 群级配置覆盖
    group_config = file_db.get("entertainment_user_limit_config", {}).get(group_id_str, {})
    if limit_type == 'guess':
        cooldown = group_config.get('guess_cooldown', default_cooldown)
        daily_limit = group_config.get('guess_daily_limit', default_daily)
    else:  # gacha
        cooldown = group_config.get('gacha_cooldown', default_cooldown)
        daily_limit = group_config.get('gacha_daily_limit', default_daily)
    
    return {'cooldown': cooldown, 'daily_limit': daily_limit}


def is_user_limit_enabled(group_id: int) -> bool:
    """检查群是否启用了用户限制"""
    group_id_str = str(group_id) if group_id else "global"
    enabled_map = file_db.get("entertainment_user_limit_enabled", {})
    # 先检查群级配置，没有则使用全局配置
    if group_id_str in enabled_map:
        return enabled_map[group_id_str]
    return enabled_map.get("global", False)


def set_user_limit_enabled(group_id: int, enabled: bool, is_global: bool = False):
    """设置用户限制开关"""
    key = "global" if is_global else str(group_id)
    enabled_map = file_db.get("entertainment_user_limit_enabled", {})
    enabled_map[key] = enabled
    file_db.set("entertainment_user_limit_enabled", enabled_map)


def check_user_entertainment_limit(ctx: SekaiHandlerContext, limit_type: str):
    """
    检查用户娱乐功能限制（仅检查，不记录使用）
    limit_type: 'guess' 或 'gacha'
    """
    # 私聊不限制
    if not ctx.group_id:
        return
    
    # 超管不限制
    if check_superuser(ctx.event):
        return
    
    # 检查限制是否启用
    if not is_user_limit_enabled(ctx.group_id):
        return
    
    group_id_str = str(ctx.group_id)
    user_id_str = str(ctx.user_id)
    now = datetime.now()
    today = now.strftime("%Y-%m-%d")
    
    # 获取配置
    config = get_user_limit_config(ctx.group_id, limit_type)
    cooldown = config['cooldown']
    daily_limit = config['daily_limit']
    
    # 检查日期是否变更，需要重置
    usage_date = file_db.get("entertainment_user_usage_date", None)
    if usage_date != today:
        file_db.set("entertainment_user_usage", {})
        file_db.set("entertainment_user_usage_date", today)
    
    # 获取用户使用记录
    all_usage = file_db.get("entertainment_user_usage", {})
    group_usage = all_usage.get(group_id_str, {})
    type_usage = group_usage.get(limit_type, {})
    user_usage = type_usage.get(user_id_str, {"last_use": 0, "daily_count": 0})
    
    last_use_ts = user_usage.get("last_use", 0)
    daily_count = user_usage.get("daily_count", 0)
    
    # 检查冷却
    if last_use_ts > 0:
        elapsed = now.timestamp() - last_use_ts
        if elapsed < cooldown:
            remain = int(cooldown - elapsed)
            minutes = remain // 60
            seconds = remain % 60
            type_name = "娱乐" if limit_type == "guess" else "抽卡"
            if minutes > 0:
                raise ReplyException(f"{type_name}功能冷却中，请等待{minutes}分{seconds}秒后再试")
            else:
                raise ReplyException(f"{type_name}功能冷却中，请等待{seconds}秒后再试")
    
    # 检查每日次数
    if daily_count >= daily_limit:
        type_name = "娱乐" if limit_type == "guess" else "抽卡"
        raise ReplyException(f"你今日{type_name}功能使用次数已达上限{daily_limit}次")


def record_user_entertainment_usage(ctx: SekaiHandlerContext, limit_type: str):
    """
    记录用户娱乐功能使用（成功使用后调用）
    limit_type: 'guess' 或 'gacha'
    """
    # 私聊不记录
    if not ctx.group_id:
        return
    
    # 超管不记录
    if check_superuser(ctx.event):
        return
    
    # 限制未启用不记录
    if not is_user_limit_enabled(ctx.group_id):
        return
    
    group_id_str = str(ctx.group_id)
    user_id_str = str(ctx.user_id)
    now = datetime.now()
    
    # 获取用户使用记录
    all_usage = file_db.get("entertainment_user_usage", {})
    group_usage = all_usage.get(group_id_str, {})
    type_usage = group_usage.get(limit_type, {})
    user_usage = type_usage.get(user_id_str, {"last_use": 0, "daily_count": 0})
    
    # 更新使用记录
    user_usage["last_use"] = now.timestamp()
    user_usage["daily_count"] = user_usage.get("daily_count", 0) + 1
    type_usage[user_id_str] = user_usage
    group_usage[limit_type] = type_usage
    all_usage[group_id_str] = group_usage
    file_db.set("entertainment_user_usage", all_usage)


def get_user_entertainment_stats(group_id: int) -> list:
    """
    获取群娱乐功能使用统计排行
    返回 [(user_id, guess_count, gacha_count, total_count), ...]
    """
    group_id_str = str(group_id)
    today = datetime.now().strftime("%Y-%m-%d")
    
    # 检查日期
    usage_date = file_db.get("entertainment_user_usage_date", None)
    if usage_date != today:
        return []
    
    all_usage = file_db.get("entertainment_user_usage", {})
    group_usage = all_usage.get(group_id_str, {})
    
    # 合并统计
    user_stats = {}
    for limit_type in ['guess', 'gacha']:
        type_usage = group_usage.get(limit_type, {})
        for user_id_str, usage in type_usage.items():
            if user_id_str not in user_stats:
                user_stats[user_id_str] = {'guess': 0, 'gacha': 0}
            user_stats[user_id_str][limit_type] = usage.get('daily_count', 0)
    
    # 转换为列表并排序
    result = []
    for user_id_str, stats in user_stats.items():
        total = stats['guess'] + stats['gacha']
        result.append((int(user_id_str), stats['guess'], stats['gacha'], total))
    
    result.sort(key=lambda x: x[3], reverse=True)
    return result


GUESS_INTERVAL = timedelta(seconds=1)
HINT_KEYWORDS = ['提示']
STOP_KEYWORDS = ['结束猜', '停止猜', '结束听', '停止听']

@dataclass
class ImageRandomCropOptions:
    rate_min: float
    rate_max: float
    flip_prob: float = 0.
    inv_prob: float = 0.
    gray_prob: float = 0.
    rgb_shuffle_prob: float = 0.
    at_least_one_effect: bool = False

    def get_effect_tip_text(self):
        effects = []
        if self.flip_prob > 0: effects.append("翻转")
        if self.inv_prob > 0: effects.append("反色")
        if self.gray_prob > 0: effects.append("灰度")
        if self.rgb_shuffle_prob > 0: effects.append("RGB打乱")
        if len(effects) == 0: return ""
        return f"（概率出现{'、'.join(effects)}效果）"

@dataclass
class ChartRandomClipOptions:
    rate_min: float
    rate_max: float
    mirror_prob: float = 0.

    def get_effect_tip_text(self):
        effects = []
        if self.mirror_prob > 0: effects.append("镜像")
        if len(effects) == 0: return ""
        return f"（概率出现{'、'.join(effects)}效果）"


GUESS_COVER_TIMEOUT = timedelta(seconds=60) 
GUESS_COVER_DIFF_OPTIONS = {
    'easy':     ImageRandomCropOptions(0.4, 0.5),
    'normal':   ImageRandomCropOptions(0.3, 0.5),
    'hard':     ImageRandomCropOptions(0.2, 0.3),
    'expert':   ImageRandomCropOptions(0.1, 0.3),
    'master':   ImageRandomCropOptions(0.1, 0.15),
    'append':   ImageRandomCropOptions(0.2, 0.5, flip_prob=0.4, inv_prob=0.4, gray_prob=0.4, rgb_shuffle_prob=0.4, at_least_one_effect=True),
}

GUESS_CHART_TIMEOUT = timedelta(seconds=60)
GUESS_CHART_DIFF_OPTIONS = {
    'easy':     ChartRandomClipOptions(0.4, 0.4),
    'normal':   ChartRandomClipOptions(0.3, 0.4),
    'hard':     ChartRandomClipOptions(0.1, 0.3),
    'expert':   ChartRandomClipOptions(0.1, 0.2),
    'master':   ChartRandomClipOptions(0.05, 0.1),
}

GUESS_CARD_TIMEOUT = timedelta(seconds=60)
GUESS_CARD_DIFF_OPTIONS = {
    'easy':     ImageRandomCropOptions(0.5, 0.5),
    'normal':   ImageRandomCropOptions(0.4, 0.5),
    'hard':     ImageRandomCropOptions(0.3, 0.4),
    'expert':   ImageRandomCropOptions(0.2, 0.3),
    'master':   ImageRandomCropOptions(0.1, 0.1),
    'append':   ImageRandomCropOptions(0.2, 0.3, flip_prob=0.5, inv_prob=0.5, gray_prob=0.5, rgb_shuffle_prob=0.5, at_least_one_effect=True),
}
GUESS_CARD_CID_LIMIT = 10

GUESS_MUSIC_TIMEOUT = timedelta(seconds=60)
GUESS_MUSIC_DIFF_OPTIONS = {
    'easy':     (15.0, False),
    'normal':   (10.0, False),
    'hard':     (7.5, False),
    'expert':   (5.0, False),
    'master':   (2.0, False),
    'append':   (10.0, True),
}


# ======================= 处理逻辑 ======================= #

# 检查次数限制
def check_daily_entertainment_limit(ctx: SekaiHandlerContext):
    group_id = str(ctx.group_id)
    daily_limits = file_db.get(f"entertainment_daily_limits", {})
    daily_usages = file_db.get(f"entertainment_daily_usages", {})
    usage_date = file_db.get(f"entertainment_usage_date", None)
    date = datetime.now().strftime("%Y-%m-%d")
    if usage_date != date:
        daily_usages = {}
        usage_date = date
    limit = daily_limits.get(group_id, DEFAULT_ENTERTAINMENT_DAILY_LIMIT)
    usage = daily_usages.get(group_id, 0)
    if usage >= limit:
        raise ReplyException(f"本群今日娱乐功能使用次数已达上限{limit}次")
    daily_usages[group_id] = usage + 1
    file_db.set(f"entertainment_daily_usages", daily_usages)
    file_db.set(f"entertainment_usage_date", usage_date)

@dataclass
class GuessContext:
    ctx: SekaiHandlerContext
    guess_type: str
    group_id: int
    start_time: Optional[datetime] = None
    end_time: Optional[datetime] = None
    user_id: Optional[int] = None
    msg_id: Optional[int] = None
    text: Optional[str] = None
    guess_success: bool = False
    used_hint_types: Set[str] = field(default_factory=set)
    data: Dict[str, Any] = field(default_factory=dict)

    async def asend_msg(self, msg: str):
        return await self.ctx.asend_msg(msg)
    
    async def asend_reply_msg(self, msg: str):
        msg = f"[CQ:reply,id={self.msg_id}]{msg}"
        return await self.ctx.asend_msg(msg)
    
guess_resp_queues: Dict[int, Dict[str, asyncio.Queue[GroupMessageEvent]]] = {}
uid_last_guess_time: Dict[int, datetime] = {}

# 记录当前猜x的消息事件
@before_record_hook
async def get_guess_resp_event(bot: Bot, event: GroupMessageEvent):
    if not is_group_msg(event): return
    if event.user_id == int(bot.self_id): return
    if check_in_blacklist(event.user_id): return
    if event.get_plaintext().startswith("/"): return
    gid = event.group_id
    queues = guess_resp_queues.get(gid, {})
    for q in queues.values():
        q.put_nowait(event)

# 开始猜x
async def start_guess(ctx: SekaiHandlerContext, guess_type: str, timeout: timedelta, start_fn, check_fn, stop_fn, hint_fn):
    gid = ctx.group_id
    current_guesses = list(guess_resp_queues.get(gid, {}).keys())
    current_guess = current_guesses[0] if len(current_guesses) > 0 else None
    assert_and_reply(not current_guess, f"当前{current_guess}正在进行")
    await ctx.block(f"{gid}", timeout=0)

    if gid not in guess_resp_queues:
        guess_resp_queues[gid] = {}
    guess_resp_queues[gid][guess_type] = asyncio.Queue()

    try:
        logger.info(f"群聊 {gid} 开始{guess_type}，timeout={timeout.total_seconds()}s")

        gctx = GuessContext(
            ctx=ctx, 
            guess_type=guess_type, 
            group_id=gid
        )
        await start_fn(gctx)
        
        # 成功开始猜x后才记录使用
        record_user_entertainment_usage(ctx, 'guess')

        gctx.start_time = datetime.now()
        gctx.end_time = datetime.now() + timeout
    
        while True:
            try:
                rest_time = gctx.end_time - datetime.now()
                if rest_time.total_seconds() <= 0:
                    raise asyncio.TimeoutError
                event = await asyncio.wait_for(
                    guess_resp_queues[gid][guess_type].get(), 
                    timeout=rest_time.total_seconds()
                )
                uid, mid, text = event.user_id, event.message_id, event.get_plaintext()
                time = datetime.fromtimestamp(event.time)
                if time - uid_last_guess_time.get(uid, datetime.min) < GUESS_INTERVAL:
                    continue
                uid_last_guess_time[uid] = time
                # logger.info(f"群聊 {gid} 收到{guess_type}消息: uid={uid}, text={text}")

                gctx.user_id = uid
                gctx.msg_id = mid
                gctx.text = text

                if any([kw in text for kw in HINT_KEYWORDS]):
                    await hint_fn(gctx)
                    continue

                if any([kw in text for kw in STOP_KEYWORDS]):
                    await stop_fn(gctx)
                    return

                await check_fn(gctx)
                if gctx.guess_success:
                    break

            except asyncio.TimeoutError:
                await stop_fn(gctx)
                return
    finally:
        logger.info(f"群聊 {gid} 停止{guess_type}")
        if gid in guess_resp_queues and guess_type in guess_resp_queues[gid]:
            del guess_resp_queues[gid][guess_type]
        # 主动清理图片数据
        gctx.data.clear()
        
        # 触发内存整理
        try:
            from ...memory_optimizer import perform_memory_trim
            # 在进程池中执行以避免阻塞事件循环
            await run_in_pool(perform_memory_trim, True)
        except Exception as e:
            logger.warning(f"触发内存整理失败: {e}")
# 随机裁剪图片到 w=[w*rate_min, w*rate_max], h=[h*rate_min, h*rate_max]
async def random_crop_image(image: Image.Image, options: ImageRandomCropOptions) -> Image.Image:
    image = image.convert("RGB")
    w, h = image.size
    w_rate = random.uniform(options.rate_min, options.rate_max)
    h_rate = random.uniform(options.rate_min, options.rate_max)
    w_crop = int(w * w_rate)
    h_crop = int(h * h_rate)
    x = random.randint(0, w - w_crop)
    y = random.randint(0, h - h_crop)
    ret = image.crop((x, y, x + w_crop, y + h_crop))

    flip, inv, gray, rgb_shuffle = False, False, False, False
    while True:
        flip = random.random() < options.flip_prob
        inv = random.random() < options.inv_prob
        gray = random.random() < options.gray_prob
        rgb_shuffle = random.random() < options.rgb_shuffle_prob
        if not options.at_least_one_effect or any([flip, inv, gray, rgb_shuffle]):
            break
        
    if flip:
        if random.random() < 0.5:
            ret = ret.transpose(Transpose.FLIP_LEFT_RIGHT)
        else:
            ret = ret.transpose(Transpose.FLIP_TOP_BOTTOM)
    if inv:
        ret = ImageOps.invert(ret)
    if gray:
        ret = ImageOps.grayscale(ret).convert("RGB")
    if rgb_shuffle:
        channels = list(range(3))
        random.shuffle(channels)
        ret = ret.split()
        ret = Image.merge("RGB", (ret[channels[0]], ret[channels[1]], ret[channels[2]]))
    return ret

# 随机歌曲，返回（歌曲数据，封面缩略图cq码，资源类型）
@retry(stop=stop_after_attempt(3), reraise=True)
async def random_music(ctx: SekaiHandlerContext, res_type: str) -> Tuple[Dict, Image.Image, Any]:
    assert res_type in ['cover', 'audio']
    musics = await ctx.md.musics.get()
    music = random.choice(musics)
    assert datetime.now() > datetime.fromtimestamp(music['publishedAt'] / 1000)
    asset_name = music['assetbundleName']
    cover_img = await ctx.rip.img(f"music/jacket/{asset_name}_rip/{asset_name}.png", allow_error=False)
    cover_thumb_cq = await get_image_cq(cover_img.resize((200, 200)), low_quality=True)
    if res_type == 'cover':
        return music, cover_thumb_cq, cover_img.resize((512, 512))
    elif res_type == 'audio':
        # 随机一个版本音频
        vocals = await ctx.md.music_vocals.find_by('musicId', music['id'], mode='all')
        vocal_assetname = random.choice(vocals)['assetbundleName']
        audio_path = await ctx.rip.get_asset_cache_path(f"music/long/{vocal_assetname}/{vocal_assetname}.mp3")
        return music, cover_thumb_cq, audio_path

# 发送猜曲提示
async def send_guess_music_hint(gctx: GuessContext):
    music = gctx.data['music']
    music_diff = await get_music_diff_info(gctx.ctx, music['id'])

    hint_types = ['ma_diff', 'title_first', 'title_last']
    if music_diff.has_append: hint_types.append('apd_diff')
    hint_types = [t for t in hint_types if t not in gctx.used_hint_types] 
    if len(hint_types) == 0:
        await gctx.asend_reply_msg("没有更多提示了！")
        return
    hint_type = random.choice(hint_types)

    msg = f"提示："
    if hint_type == 'title_first':
        msg += f"歌曲标题以\"{music['title'][0]}\"开头"
    elif hint_type == 'title_last':
        msg += f"歌曲标题以\"{music['title'][-1]}\"结尾"
    elif hint_type == 'ma_diff':
        msg += f"MASTER Lv.{music_diff.level['master']}"
    elif hint_type == 'apd_diff':
        msg += f"APPEND Lv.{music_diff.level['append']}"
    elif hint_type == 'month':
        time = datetime.fromtimestamp(music['publishedAt'] / 1000.)
        msg += f"发布时间为{time.year}年{time.month}月"

    gctx.used_hint_types.add(hint_type)    
    await gctx.asend_msg(msg)

# 获取卡面标题
async def get_card_title(ctx: SekaiHandlerContext, card: Dict, after_training: bool) -> str:
    title = f"【{card['id']}】"
    rarity = card['cardRarityType']
    if rarity == 'rarity_1': title += "⭐"
    elif rarity == 'rarity_2': title += "⭐⭐"
    elif rarity == 'rarity_3': title += "⭐⭐⭐"
    elif rarity == 'rarity_4': title += "⭐⭐⭐⭐"
    elif rarity == 'rarity_birthday': title += "🎀"
    title += " " + await get_character_name_by_id(ctx, card['characterId'])
    title += f" - {card['prefix']}"
    if rarity in ['rarity_3', 'rarity_4']:
        if after_training:  title += "（特训后）"
        else:               title += "（特训前）"
    return title

# 随机卡面，返回卡牌数据、卡面图片、是否特训
@retry(stop=stop_after_attempt(3), reraise=True)
async def random_card(ctx: SekaiHandlerContext) -> Tuple[Dict, Image.Image, str]:
    cards = await ctx.md.cards.get()
    while True:
        card = random.choice(cards)
        if datetime.fromtimestamp(card['releaseAt'] / 1000) > datetime.now():
            continue
        if card['cardRarityType'] in ['rarity_3', 'rarity_4', 'rarity_birthday']:
            break
    if not has_after_training(card):
        after_training = False
    elif only_has_after_training(card):
        after_training = True
    else:
        after_training = random.choice([True, False])
    card_img = await get_card_image(ctx, card['id'], after_training=after_training, allow_error=False)
    card_img = resize_keep_ratio(card_img, 1024 * 512, mode='wxh')
    return card, card_img, after_training

# 发送猜卡面提示
async def send_guess_card_hint(gctx: GuessContext):
    card = gctx.data['card']
    after_training = gctx.data['after_training']

    hint_types = ['name', 'rarity_and_attr', 'unit']
    hint_types = [t for t in hint_types if t not in gctx.used_hint_types]
    if len(hint_types) == 0:
        await gctx.asend_reply_msg("没有更多提示了！")
        return
    hint = random.choice(hint_types)

    msg = f"提示："
    if hint == 'name':
        msg += f"标题为\"{card['prefix']}\""
    elif hint == 'after_training':
        if after_training:  msg += "特训后"
        else:               msg += "特训前"
    elif hint == 'rarity_and_attr':
        rarity = card['cardRarityType']
        if rarity == 'rarity_1': msg += "1星"
        elif rarity == 'rarity_2': msg += "2星"
        elif rarity == 'rarity_3': msg += "3星"
        elif rarity == 'rarity_4': msg += "4星"
        elif rarity == 'rarity_birthday': msg += "生日卡"
        msg += "&"
        attr = card['attr']
        if attr == 'cool': msg += "蓝星"
        elif attr == 'happy': msg += "橙心"
        elif attr == 'mysterious': msg += "紫月"
        elif attr == 'cute': msg += "粉花"
        elif attr == 'pure': msg += "绿草"
    elif hint == 'month':
        time = datetime.fromtimestamp(card['releaseAt'] / 1000.)
        msg += f"发布时间为{time.year}年{time.month}月"
    elif hint == 'unit':
        unit = await get_unit_by_card_id(gctx.ctx, card['id'])
        if unit == 'light_sound': msg += "ln"
        elif unit == 'idol': msg += "mmj"
        elif unit == 'street': msg += "vbs"
        elif unit == 'theme_park': msg += "ws"
        elif unit == 'school_refusal': msg += "25时"
        elif unit == 'piapro': msg += "vs"

    gctx.used_hint_types.add(hint)
    await gctx.asend_msg(msg)

# 随机裁剪音频+反转
async def random_clip_audio(input_path: str, save_path: str, length: float, reverse: bool = False, clip_start=20.0, clip_end=10.0):
    audio = pydub.AudioSegment.from_file(input_path)
    length = int(length * 1000)
    start = random.randint(int(clip_start * 1000), len(audio) - length - int(clip_end * 1000))
    clip = audio[start:start + length]
    if reverse:
        clip = clip.reverse()
    clip.export(save_path, format='mp3')

# 获取猜曲检查函数
def get_guess_music_check_fn(guess_type: str):
    async def check_fn(gctx: GuessContext):
        music, cover_thumb = gctx.data['music'], gctx.data['cover_thumb']
        ret: MusicSearchResult = await search_music(
            gctx.ctx, 
            gctx.text, 
            MusicSearchOptions(
                use_id=False,
                use_nidx=False,
                use_emb=False, 
                raise_when_err=False, 
                verbose=False
            ))
        if ret.music is None:
            return
        if ret.music['id'] == music['id']:
            await gctx.asend_reply_msg(f"你猜对了！\n【{music['id']}】{music['title']}{cover_thumb}")
            gctx.guess_success = True
    return check_fn

# 获取猜曲停止函数
def get_guess_music_stop_fn(guess_type: str):
    async def stop_fn(gctx: GuessContext):
        music, cover_thumb = gctx.data['music'], gctx.data['cover_thumb']
        await gctx.asend_msg(f"{guess_type}结束，正确答案：\n【{music['id']}】{music['title']}{cover_thumb}")
    return stop_fn


# ======================= 指令处理 ======================= #

# 设置娱乐功能次数限制
pjsk_entertainment_limit = SekaiCmdHandler([
    "/pjsk entertainment limit", "/pjsk_entertainment_limit", 
    "/pjsk娱乐功能上限", "/pel",
], regions=['jp'])
pjsk_entertainment_limit.check_cdrate(cd).check_wblist(gbl).check_superuser()
@pjsk_entertainment_limit.handle()
async def _(ctx: SekaiHandlerContext):
    limit = int(ctx.get_args().strip())
    assert_and_reply(limit >= 0, "次数限制必须大于等于0")
    group_id = str(ctx.group_id)
    daily_limits = file_db.get(f"entertainment_daily_limits", {})
    daily_limits[group_id] = limit
    file_db.set(f"entertainment_daily_limits", daily_limits)
    await ctx.asend_reply_msg(f"已设置本群娱乐功能次数限制为每日{limit}次")


# 查看娱乐功能次数
pjsk_entertainment_limit_check = SekaiCmdHandler([
    "/pjsk entertainment count", "/pjsk_entertainment_count",
    "/pjsk娱乐功能次数", "/pec",
], regions=['jp'])
pjsk_entertainment_limit_check.check_cdrate(cd).check_wblist(gbl)
@pjsk_entertainment_limit_check.handle()
async def _(ctx: SekaiHandlerContext):
    group_id = str(ctx.group_id)
    daily_limits = file_db.get(f"entertainment_daily_limits", {})
    daily_usages = file_db.get(f"entertainment_daily_usages", {})
    usage_date = file_db.get(f"entertainment_usage_date", None)
    date = datetime.now().strftime("%Y-%m-%d")
    if usage_date != date:
        daily_usages = {}
        usage_date = date
    limit = daily_limits.get(group_id, DEFAULT_ENTERTAINMENT_DAILY_LIMIT)
    usage = daily_usages.get(group_id, 0)
    await ctx.asend_reply_msg(f"本群今日娱乐功能使用次数：{usage}/{limit}次")


# 猜曲封
pjsk_guess_cover = SekaiCmdHandler([
    "/pjsk guess cover", "/pjsk_guess_cover", 
    "/pjsk猜曲封", "/pjsk猜曲绘", "/猜曲绘", "/猜曲封",
], regions=['jp'])
pjsk_guess_cover.check_cdrate(cd).check_wblist(gbl)
@pjsk_guess_cover.handle()
async def _(ctx: SekaiHandlerContext):
    check_daily_entertainment_limit(ctx)
    check_user_entertainment_limit(ctx, 'guess')

    args = ctx.get_args().strip()
    diff, args = extract_diff(args, default='expert')
    assert_and_reply(diff in GUESS_COVER_DIFF_OPTIONS, f"可选难度：{', '.join(GUESS_COVER_DIFF_OPTIONS.keys())}")

    async def start_fn(gctx: GuessContext):
        music, cover_thumb, cover_img = await random_music(gctx.ctx, 'cover')
        logger.info(f"群聊 {gctx.group_id} 猜曲绘目标: {music['id']}")
        crop_img = await random_crop_image(cover_img, GUESS_COVER_DIFF_OPTIONS[diff])
        msg = await get_image_cq(crop_img)
        msg += f"{diff.upper()}模式猜曲绘{GUESS_COVER_DIFF_OPTIONS[diff].get_effect_tip_text()}"
        msg += f"，限时{int(GUESS_COVER_TIMEOUT.total_seconds())}秒"
        msg += "（无需回复，直接发送歌名/id/别名）"
        await gctx.asend_msg(msg)
        gctx.data['music'] = music
        gctx.data['cover_thumb'] = cover_thumb

    await start_guess(
        ctx, '猜曲绘', GUESS_COVER_TIMEOUT, start_fn, 
        get_guess_music_check_fn('猜曲绘'), get_guess_music_stop_fn('猜曲绘'),
        send_guess_music_hint
    )


# 猜谱面
pjsk_guess_chart = SekaiCmdHandler([
    "/pjsk guess chart", "/pjsk_guess_chart", 
    "/pjsk猜谱面", "/猜谱面", "/pjsk猜铺面", "/猜铺面",
], regions=['jp'])
pjsk_guess_chart.check_cdrate(cd).check_wblist(gbl)
@pjsk_guess_chart.handle()
async def _(ctx: SekaiHandlerContext):
    check_daily_entertainment_limit(ctx)
    check_user_entertainment_limit(ctx, 'guess')

    args = ctx.get_args().strip()
    diff, args = extract_diff(args, default='expert')
    assert_and_reply(diff in GUESS_CHART_DIFF_OPTIONS, f"可选难度：{', '.join(GUESS_CHART_DIFF_OPTIONS.keys())}")

    async def start_fn(gctx: GuessContext):
        music, cover_thumb, _ = await random_music(gctx.ctx, 'cover')
        logger.info(f"群聊 {gctx.group_id} 猜谱面目标: {music['id']}")
        diff_info = await get_music_diff_info(gctx.ctx, music['id'])
        chart_diff = random.choice(['master', 'append']) if diff_info.has_append else 'master'
        chart_lv = diff_info.level[chart_diff]
        rate = random.uniform(
            GUESS_CHART_DIFF_OPTIONS[diff].rate_min, 
            GUESS_CHART_DIFF_OPTIONS[diff].rate_max
        )
        clip_chart = await generate_music_chart(
            gctx.ctx, music['id'], chart_diff, need_reply=False, 
            random_clip_length_rate=rate, style_sheet='guess',
            use_cache=False
        )
        msg = await get_image_cq(clip_chart)
        msg += f"{diff.upper()}模式猜谱面{GUESS_CHART_DIFF_OPTIONS[diff].get_effect_tip_text()}"
        msg += f"（谱面难度可能为MASTER或APPEND），限时{int(GUESS_CHART_TIMEOUT.total_seconds())}秒"
        msg += "（无需回复，直接发送歌名/id/别名）"
        await gctx.asend_msg(msg)
        gctx.data['music'] = music
        gctx.data['cover_thumb'] = cover_thumb
        gctx.data['chart_diff'] = chart_diff
        gctx.data['chart_lv'] = chart_lv

    await start_guess(
        ctx, '猜谱面', GUESS_CHART_TIMEOUT, start_fn, 
        get_guess_music_check_fn('猜谱面'), get_guess_music_stop_fn('猜谱面'),
        send_guess_music_hint
    )


# 猜卡面
pjsk_guess_card = SekaiCmdHandler([
    "/pjsk guess card", "/pjsk_guess_card", 
    "/pjsk猜卡面", "/猜卡面", "/pjsk猜卡", "/猜卡",
], regions=['jp'])
pjsk_guess_card.check_cdrate(cd).check_wblist(gbl)
@pjsk_guess_card.handle()
async def _(ctx: SekaiHandlerContext):
    check_daily_entertainment_limit(ctx)
    check_user_entertainment_limit(ctx, 'guess')

    args = ctx.get_args().strip()
    diff, args = extract_diff(args, default='expert')
    assert_and_reply(diff in GUESS_CARD_DIFF_OPTIONS, f"可选难度：{', '.join(GUESS_CARD_DIFF_OPTIONS.keys())}")

    async def start_fn(gctx: GuessContext):
        card, card_img, after_training = await random_card(gctx.ctx)
        logger.info(f"群聊 {gctx.group_id} 猜卡面目标: {card['id']}")
        crop_img = await random_crop_image(card_img, GUESS_CARD_DIFF_OPTIONS[diff])
        msg = await get_image_cq(crop_img)
        msg += f"{diff.upper()}模式猜卡面{GUESS_CARD_DIFF_OPTIONS[diff].get_effect_tip_text()}"
        msg += f"，限时{int(GUESS_CARD_TIMEOUT.total_seconds())}秒"
        msg += "（无需回复，直接发送角色简称例如ick,saki）"
        await gctx.asend_msg(msg)
        gctx.data['card'] = card
        gctx.data['card_img'] = card_img
        gctx.data['after_training'] = after_training
        gctx.data['guessed'] = set()

    async def check_fn(gctx: GuessContext):
        card, card_img, after_training = gctx.data['card'], gctx.data['card_img'], gctx.data['after_training']
        cid = get_cid_by_nickname(gctx.text)
        if cid is not None:
            gctx.data['guessed'].add(cid)
        if cid == card["characterId"]:
            await gctx.asend_reply_msg(f"你猜对了！\n{await get_card_title(gctx.ctx, card, after_training)}")
            await gctx.asend_msg(await get_image_cq(card_img, low_quality=True))
            gctx.guess_success = True
        if len(gctx.data['guessed']) > GUESS_CARD_CID_LIMIT:
            await gctx.asend_msg(f"猜卡面失败，正确答案：\n{await get_card_title(ctx, card, after_training)}")
            await gctx.asend_msg(await get_image_cq(card_img, low_quality=True))
            gctx.guess_success = True
    
    async def stop_fn(gctx: GuessContext):
        card, card_img, after_training = gctx.data['card'], gctx.data['card_img'], gctx.data['after_training']
        await gctx.asend_msg(f"猜卡面结束，正确答案：\n{await get_card_title(ctx, card, after_training)}")
        await gctx.asend_msg(await get_image_cq(card_img, low_quality=True))

    await start_guess(ctx, '猜卡面', GUESS_CARD_TIMEOUT, start_fn, check_fn, stop_fn, send_guess_card_hint)


# 听歌识曲
pjsk_guess_music = SekaiCmdHandler([
    "/pjsk guess music", "/pjsk_guess_music", 
    "/听歌识曲", "/pjsk听歌识曲", "/猜歌", "/pjsk猜歌", "/猜曲", "/pjsk猜曲",
], regions=['jp'])
pjsk_guess_music.check_cdrate(cd).check_wblist(gbl)
@pjsk_guess_music.handle()
async def _(ctx: SekaiHandlerContext):
    check_daily_entertainment_limit(ctx)
    check_user_entertainment_limit(ctx, 'guess')

    with TempFilePath('mp3', remove_after=timedelta(minutes=3)) as clipped_audio_path:
        args = ctx.get_args().strip()
        diff, args = extract_diff(args, default='expert')
        assert_and_reply(diff in GUESS_MUSIC_DIFF_OPTIONS, f"可选难度：{', '.join(GUESS_MUSIC_DIFF_OPTIONS.keys())}")

        async def start_fn(gctx: GuessContext):
            music, cover_thumb, audio_path = await random_music(gctx.ctx, 'audio')
            logger.info(f"群聊 {gctx.group_id} 听歌识曲目标: {music['id']}")
            await random_clip_audio(
                audio_path, clipped_audio_path, 
                length=GUESS_MUSIC_DIFF_OPTIONS[diff][0], 
                reverse=GUESS_MUSIC_DIFF_OPTIONS[diff][1],
            )
            tip_text = "（音频已反转）" if GUESS_MUSIC_DIFF_OPTIONS[diff][1] else ""
            msg = f"{diff.upper()}模式听歌识曲{tip_text}"
            msg += f"，限时{int(GUESS_MUSIC_TIMEOUT.total_seconds())}秒"
            msg += "（无需回复，直接发送歌名/id/别名）"
            await gctx.asend_msg(msg)
            await gctx.asend_msg(f"[CQ:record,file=file://{os.path.abspath(clipped_audio_path)}]")
            gctx.data['music'] = music
            gctx.data['cover_thumb'] = cover_thumb

        await start_guess(
            ctx, '听歌识曲', GUESS_MUSIC_TIMEOUT, start_fn, 
            get_guess_music_check_fn('听歌识曲'), get_guess_music_stop_fn('听歌识曲'),
            send_guess_music_hint
        )


# 模拟抽卡
pjsk_spin_gacha = SekaiCmdHandler([
    "/单抽", "/十连", *[f"/{x}连" for x in (10, 50, 100, 150, 200)],
])
pjsk_spin_gacha.check_cdrate(cd).check_wblist(gbl)
@pjsk_spin_gacha.handle()
async def _(ctx: SekaiHandlerContext):
    check_daily_entertainment_limit(ctx)
    check_user_entertainment_limit(ctx, 'gacha')

    args = ctx.get_args().strip()
    if not args:
        args = "-1"
    gacha = await parse_search_gacha_args(ctx, args)
    assert_and_reply(gacha, f"参数错误，{SINGLE_GACHA_HELP}")

    if "单抽" in ctx.trigger_cmd:
        count = 1
    elif "十连" in ctx.trigger_cmd:
        count = 10
    else:
        count = int(ctx.trigger_cmd.split("连")[0][1:])

    cards = await spin_gacha(ctx, gacha, count)
    
    # 成功抽卡后才记录使用
    record_user_entertainment_usage(ctx, 'gacha')
    
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_gacha_spin_image(ctx, gacha, cards), 
        low_quality=True
    ))


# ======================= 用户限制管理指令 ======================= #

# 开关/配置用户娱乐限制
pjsk_user_limit = SekaiCmdHandler([
    "/pjsk娱乐限制", "/pjsk user limit", "/pul",
], regions=['jp'])
pjsk_user_limit.check_cdrate(cd).check_wblist(gbl)
@pjsk_user_limit.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    
    # 检查权限
    is_admin = await check_group_admin_or_super(ctx)
    is_super = check_superuser(ctx.event)
    
    # 解析参数
    is_global = args.startswith("全局")
    if is_global:
        args = args[2:].strip()
        if not is_super:
            return await ctx.asend_reply_msg("只有超管可以设置全局配置")
    
    # 无参数 - 查看状态
    if not args:
        group_id = ctx.group_id
        enabled = is_user_limit_enabled(group_id)
        guess_config = get_user_limit_config(group_id, 'guess')
        gacha_config = get_user_limit_config(group_id, 'gacha')
        
        msg = f"本群娱乐功能用户限制: {'✅开启' if enabled else '❌关闭'}\n"
        msg += f"猜x类: 冷却{guess_config['cooldown']//60}分钟, 每日{guess_config['daily_limit']}次\n"
        msg += f"抽卡类: 冷却{gacha_config['cooldown']//60}分钟, 每日{gacha_config['daily_limit']}次\n"
        msg += f"\n设置: /pjsk娱乐限制 开/关/冷却/每日/抽卡冷却/抽卡每日"
        return await ctx.asend_fold_msg(msg)
    
    # 开关
    if args in ['开', 'on', '开启']:
        if not is_admin:
            return await ctx.asend_reply_msg("需要群管理员或超管权限")
        set_user_limit_enabled(ctx.group_id, True, is_global)
        scope = "全局" if is_global else "本群"
        return await ctx.asend_reply_msg(f"已开启{scope}娱乐功能用户限制")
    
    if args in ['关', 'off', '关闭']:
        if not is_admin:
            return await ctx.asend_reply_msg("需要群管理员或超管权限")
        set_user_limit_enabled(ctx.group_id, False, is_global)
        scope = "全局" if is_global else "本群"
        return await ctx.asend_reply_msg(f"已关闭{scope}娱乐功能用户限制")
    
    # 配置参数
    if not is_admin:
        return await ctx.asend_reply_msg("需要群管理员或超管权限")
    
    parts = args.split()
    if len(parts) < 2:
        return await ctx.asend_reply_msg("参数格式: 猜x冷却/猜x每日/抽卡冷却/抽卡每日 <数值>")
    
    param_type, value_str = parts[0], parts[1]
    try:
        value = int(value_str)
        assert value > 0
    except:
        return await ctx.asend_reply_msg("数值必须是正整数")
    
    # 解析参数类型
    config_key = None
    if param_type in ['冷却', 'guess_cooldown']:
        config_key = 'guess_cooldown'
        value_desc = f"娱乐功能冷却{value}秒"
    elif param_type in ['每日', 'guess_daily', '猜x次数']:
        config_key = 'guess_daily_limit'
        value_desc = f"娱乐功能每人每日{value}次"
    elif param_type in ['抽卡冷却', 'gacha_cooldown']:
        config_key = 'gacha_cooldown'
        value_desc = f"抽卡功能冷却{value}秒"
    elif param_type in ['抽卡每日', 'gacha_daily', '抽卡次数']:
        config_key = 'gacha_daily_limit'
        value_desc = f"抽卡功能每人每日{value}次"
    else:
        return await ctx.asend_reply_msg("参数格式: 冷却/每日 <数值>")
    
    # 保存配置
    key = "global" if is_global else str(ctx.group_id)
    all_config = file_db.get("entertainment_user_limit_config", {})
    if key not in all_config:
        all_config[key] = {}
    all_config[key][config_key] = value
    file_db.set("entertainment_user_limit_config", all_config)
    
    scope = "全局" if is_global else "本群"
    return await ctx.asend_reply_msg(f"已设置{scope}{value_desc}")


# 查看个人娱乐功能使用次数
pjsk_user_usage = SekaiCmdHandler([
    "/pjsk娱乐次数", "/pjsk user usage", "/peu",
], regions=['jp'])
pjsk_user_usage.check_cdrate(cd).check_wblist(gbl)
@pjsk_user_usage.handle()
async def _(ctx: SekaiHandlerContext):
    if not ctx.group_id:
        return await ctx.asend_reply_msg("私聊不受娱乐功能限制")
    
    group_id_str = str(ctx.group_id)
    user_id_str = str(ctx.user_id)
    today = datetime.now().strftime("%Y-%m-%d")
    
    # 获取配置
    guess_config = get_user_limit_config(ctx.group_id, 'guess')
    gacha_config = get_user_limit_config(ctx.group_id, 'gacha')
    
    # 检查日期
    usage_date = file_db.get("entertainment_user_usage_date", None)
    if usage_date != today:
        guess_count, gacha_count = 0, 0
    else:
        all_usage = file_db.get("entertainment_user_usage", {})
        group_usage = all_usage.get(group_id_str, {})
        guess_usage = group_usage.get('guess', {}).get(user_id_str, {})
        gacha_usage = group_usage.get('gacha', {}).get(user_id_str, {})
        guess_count = guess_usage.get('daily_count', 0)
        gacha_count = gacha_usage.get('daily_count', 0)
    
    enabled = is_user_limit_enabled(ctx.group_id)
    status = "开启" if enabled else "关闭"
    
    msg = f"今日娱乐功能使用情况（限制{status}）:\n"
    msg += f"猜x类: {guess_count}/{guess_config['daily_limit']}次\n"
    msg += f"抽卡类: {gacha_count}/{gacha_config['daily_limit']}次"
    
    return await ctx.asend_reply_msg(msg)


# 娱乐功能使用统计排行
pjsk_entertainment_stats = SekaiCmdHandler([
    "/pjsk娱乐统计", "/pjsk entertainment stats", "/pes",
], regions=['jp'])
pjsk_entertainment_stats.check_cdrate(cd).check_wblist(gbl)
@pjsk_entertainment_stats.handle()
async def _(ctx: SekaiHandlerContext):
    if not ctx.group_id:
        return await ctx.asend_reply_msg("该指令仅限群聊使用")
    
    # 检查权限
    is_admin = await check_group_admin_or_super(ctx)
    if not is_admin:
        return await ctx.asend_reply_msg("需要群管理员或超管权限")
    
    stats = get_user_entertainment_stats(ctx.group_id)
    if not stats:
        return await ctx.asend_reply_msg("今日暂无使用记录")
    
    # 限制显示前15名
    stats = stats[:15]
    
    msg = "📊 今日娱乐功能使用排行:\n"
    for i, (user_id, guess, gacha, total) in enumerate(stats):
        try:
            name = await get_group_member_name(ctx.group_id, user_id)
        except:
            name = str(user_id)
        rank = i + 1
        msg += f"{rank}. {name}: 猜x{guess}次 抽卡{gacha}次 共{total}次\n"
    
    return await ctx.asend_reply_msg(msg.strip())

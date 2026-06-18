from ...utils import *
from ..common import *
from ..handler import *
from ..draw import *
from ..gameapi import get_gameapi_config, request_gameapi
from .profile import get_player_bind_id
from .sk import (
    get_board_rank_str,
    get_board_score_str,
    get_rank_from_text,
    is_rank_text,
    SK_TEXT_QUERY_BG_COLOR,
)
from .event import get_current_event, get_event_banner_img
import aiosqlite
import asyncio
import hashlib
import json


CN_WISH_REGION = "cn"
CN_WISH_SERVER_ID = "60001"
WISH_DB_PATH = f"{SEKAI_DATA_DIR}/db/wish_cn_ranking.db"
WISH_REFRESH_HOUR_CFG = config.item("wish.refresh_hour")
WISH_CHECK_INTERVAL_CFG = config.item("wish.check_interval_seconds")
WISH_QUERY_MAX_RANK = 500
WISH_QUERY_MAX_RANK_COUNT = 20
WISH_LINE_RANKS = [
    1, 10, 20, 30, 40, 50,
    100, 200, 300, 400, 500,
    1000, 1500, 2000, 2500, 3000, 4000, 5000,
    10000, 20000, 30000, 40000, 50000,
]

_wish_conn: Optional[aiosqlite.Connection] = None
_wish_tables_created = False
_wish_sync_lock = asyncio.Lock()
_wish_columns_checked = False


def load_wish_location():
    return timezone(timedelta(hours=8))


def get_wish_now() -> datetime:
    return datetime.now(load_wish_location())


def get_wish_now_naive() -> datetime:
    return get_wish_now().replace(tzinfo=None)


def from_wish_timestamp(ts: int | float) -> datetime:
    return datetime.fromtimestamp(ts, load_wish_location()).replace(tzinfo=None)


def get_today_refresh_time(now: datetime = None) -> datetime:
    now = now or get_wish_now_naive()
    return now.replace(hour=WISH_REFRESH_HOUR_CFG.get(), minute=0, second=0, microsecond=0)


@dataclass
class WishRankingEntry:
    rank: int
    role_id: str
    role_name: str
    pt: int
    server_id: int = 0
    list_type: str = ""
    snapshot_id: Optional[int] = None
    id: Optional[int] = None

    @classmethod
    def from_api(cls, list_type: str, data: dict):
        return cls(
            rank=int(data.get("rank") or 0),
            role_id=str(data.get("role_id") or ""),
            role_name=str(data.get("role_name") or "").strip(),
            pt=int(data.get("pt") or 0),
            server_id=int(data.get("server_id") or 0),
            list_type=list_type,
        )

    @classmethod
    def from_row(cls, row):
        return cls(
            id=row[0],
            snapshot_id=row[1],
            list_type=row[2],
            rank=row[3],
            role_id=row[4],
            role_name=row[5],
            pt=row[6],
            server_id=row[7],
        )

    def to_hash_dict(self):
        return {
            "rank": self.rank,
            "role_id": self.role_id,
            "role_name": self.role_name,
            "pt": self.pt,
            "server_id": self.server_id,
            "list_type": self.list_type,
        }


@dataclass
class WishPeriodInfo:
    period_index: int
    period_total: int
    start_at: datetime
    end_at: datetime

    @classmethod
    def from_api(cls, data: dict):
        if not data:
            return None
        start_at = data.get("start_at")
        end_at = data.get("end_at")
        if not start_at or not end_at:
            return None
        return cls(
            period_index=int(data.get("period_index") or 0),
            period_total=int(data.get("period_total") or 0),
            start_at=from_wish_timestamp(start_at),
            end_at=from_wish_timestamp(end_at),
        )


@dataclass
class WishHeaderInfo:
    title: str
    current_title: str
    current_event: Optional[dict]
    current_banner: Optional[Image.Image]
    current_start_at: Optional[datetime]
    current_end_at: Optional[datetime]
    current_remaining_text: str
    period_info: Optional[WishPeriodInfo]
    period_remaining_text: str
    remaining_event_count: Optional[int]
    t500_score_text: str
    latest_rt_text: str
    next_event: Optional[dict]
    next_start_at: Optional[datetime]
    next_banner: Optional[Image.Image]


@dataclass
class WishRankingSnapshot:
    id: int
    activity_id: str
    process_id: str
    fetched_at: datetime
    total_count: int
    content_hash: str = ""
    period_info: Optional[WishPeriodInfo] = None
    ladder: List[WishRankingEntry] = field(default_factory=list)
    topN: List[WishRankingEntry] = field(default_factory=list)

    @classmethod
    def from_row(cls, row):
        period_info = None
        if len(row) >= 10 and row[6] and row[8] and row[9]:
            period_info = WishPeriodInfo(
                period_index=int(row[6] or 0),
                period_total=int(row[7] or 0),
                start_at=from_wish_timestamp(row[8]),
                end_at=from_wish_timestamp(row[9]),
            )
        return cls(
            id=row[0],
            activity_id=row[1],
            process_id=row[2],
            fetched_at=from_wish_timestamp(row[3]),
            total_count=row[4],
            content_hash=row[5],
            period_info=period_info,
        )


async def get_wish_conn(create: bool = True) -> Optional[aiosqlite.Connection]:
    create_parent_folder(WISH_DB_PATH)
    if not create and not os.path.exists(WISH_DB_PATH):
        return None

    global _wish_conn, _wish_tables_created, _wish_columns_checked
    if _wish_conn is None:
        _wish_conn = await aiosqlite.connect(WISH_DB_PATH)
        await _wish_conn.execute("PRAGMA journal_mode=WAL;")
        await _wish_conn.execute("PRAGMA foreign_keys=ON;")
        logger.info(f"连接sqlite数据库 {WISH_DB_PATH} 成功")

    if not _wish_tables_created:
        await _wish_conn.execute("""
            CREATE TABLE IF NOT EXISTS snapshot (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                activity_id TEXT NOT NULL,
                process_id TEXT NOT NULL,
                fetched_at INTEGER NOT NULL,
                total_count INTEGER NOT NULL,
                content_hash TEXT NOT NULL UNIQUE,
                period_index INTEGER,
                period_total INTEGER,
                period_start_at INTEGER,
                period_end_at INTEGER
            )
        """)
        await _wish_conn.execute("""
            CREATE TABLE IF NOT EXISTS ranking (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                snapshot_id INTEGER NOT NULL,
                list_type TEXT NOT NULL,
                rank INTEGER NOT NULL,
                role_id TEXT NOT NULL,
                role_name TEXT NOT NULL,
                pt INTEGER NOT NULL,
                server_id INTEGER NOT NULL,
                FOREIGN KEY(snapshot_id) REFERENCES snapshot(id) ON DELETE CASCADE
            )
        """)
        await _wish_conn.execute("""
            CREATE INDEX IF NOT EXISTS idx_wish_snapshot_activity_time
            ON snapshot (activity_id, fetched_at DESC, id DESC)
        """)
        await _wish_conn.execute("""
            CREATE INDEX IF NOT EXISTS idx_wish_ranking_snapshot_list_rank
            ON ranking (snapshot_id, list_type, rank)
        """)
        await _wish_conn.execute("""
            CREATE INDEX IF NOT EXISTS idx_wish_ranking_snapshot_role
            ON ranking (snapshot_id, role_id)
        """)
        await _wish_conn.commit()
        _wish_tables_created = True

    if not _wish_columns_checked:
        cursor = await _wish_conn.execute("PRAGMA table_info(snapshot)")
        rows = await cursor.fetchall()
        await cursor.close()
        columns = {row[1] for row in rows}
        alter_sqls = []
        if "period_index" not in columns:
            alter_sqls.append("ALTER TABLE snapshot ADD COLUMN period_index INTEGER")
        if "period_total" not in columns:
            alter_sqls.append("ALTER TABLE snapshot ADD COLUMN period_total INTEGER")
        if "period_start_at" not in columns:
            alter_sqls.append("ALTER TABLE snapshot ADD COLUMN period_start_at INTEGER")
        if "period_end_at" not in columns:
            alter_sqls.append("ALTER TABLE snapshot ADD COLUMN period_end_at INTEGER")
        for sql in alter_sqls:
            await _wish_conn.execute(sql)
        if alter_sqls:
            await _wish_conn.commit()
        _wish_columns_checked = True

    return _wish_conn


def get_wish_role_id(uid: str) -> str:
    return f"{uid}_{CN_WISH_SERVER_ID}"


def get_wish_entry_name(entry: WishRankingEntry) -> str:
    if entry.role_name:
        return entry.role_name
    return entry.role_id.removesuffix(f"_{CN_WISH_SERVER_ID}")


def format_wish_rank(rank: int) -> str:
    return f"T{get_board_rank_str(rank)}"


def format_wish_delta(delta: Optional[int]) -> str:
    if delta is None:
        return "-"
    prefix = "+" if delta >= 0 else "-"
    return prefix + get_board_score_str(abs(delta), precise=False)


def compute_wish_content_hash(activity_id: str, process_id: str, ladder: List[WishRankingEntry], topN: List[WishRankingEntry]) -> str:
    payload = {
        "activity_id": activity_id,
        "process_id": process_id,
        "ladder": [entry.to_hash_dict() for entry in ladder],
        "topN": [entry.to_hash_dict() for entry in topN],
    }
    content = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha1(content.encode("utf-8")).hexdigest()


async def query_wish_snapshot_list(activity_id: str = None, limit: int = 1, offset: int = 0) -> List[WishRankingSnapshot]:
    conn = await get_wish_conn(create=False)
    if not conn:
        return []

    sql = """
        SELECT
            id, activity_id, process_id, fetched_at, total_count, content_hash,
            period_index, period_total, period_start_at, period_end_at
        FROM snapshot
    """
    args = []
    if activity_id is not None:
        sql += " WHERE activity_id = ?"
        args.append(activity_id)
    sql += " ORDER BY fetched_at DESC, id DESC LIMIT ? OFFSET ?"
    args.extend([limit, offset])

    cursor = await conn.execute(sql, args)
    rows = await cursor.fetchall()
    await cursor.close()
    return [WishRankingSnapshot.from_row(row) for row in rows]


async def query_wish_entries(snapshot_id: int, list_type: str) -> List[WishRankingEntry]:
    conn = await get_wish_conn(create=False)
    if not conn:
        return []

    cursor = await conn.execute("""
        SELECT id, snapshot_id, list_type, rank, role_id, role_name, pt, server_id
        FROM ranking
        WHERE snapshot_id = ? AND list_type = ?
        ORDER BY rank
    """, (snapshot_id, list_type))
    rows = await cursor.fetchall()
    await cursor.close()
    return [WishRankingEntry.from_row(row) for row in rows]


async def load_wish_snapshot_entries(snapshot: WishRankingSnapshot) -> WishRankingSnapshot:
    snapshot.ladder = await query_wish_entries(snapshot.id, "ladder")
    snapshot.topN = await query_wish_entries(snapshot.id, "topN")
    return snapshot


async def query_wish_snapshot_by_id(snapshot_id: int, load_entries: bool = True) -> Optional[WishRankingSnapshot]:
    conn = await get_wish_conn(create=False)
    if not conn:
        return None

    cursor = await conn.execute("""
        SELECT
            id, activity_id, process_id, fetched_at, total_count, content_hash,
            period_index, period_total, period_start_at, period_end_at
        FROM snapshot
        WHERE id = ?
    """, (snapshot_id,))
    row = await cursor.fetchone()
    await cursor.close()
    if not row:
        return None

    snapshot = WishRankingSnapshot.from_row(row)
    if load_entries:
        await load_wish_snapshot_entries(snapshot)
    return snapshot


async def query_latest_wish_snapshot(activity_id: str = None, load_entries: bool = True) -> Optional[WishRankingSnapshot]:
    snapshots = await query_wish_snapshot_list(activity_id=activity_id, limit=1)
    if not snapshots:
        return None
    snapshot = snapshots[0]
    if load_entries:
        await load_wish_snapshot_entries(snapshot)
    return snapshot


async def query_prev_wish_snapshot(activity_id: str) -> Optional[WishRankingSnapshot]:
    snapshots = await query_wish_snapshot_list(activity_id=activity_id, limit=2)
    if len(snapshots) < 2:
        return None
    snapshot = snapshots[1]
    await load_wish_snapshot_entries(snapshot)
    return snapshot


def find_wish_entry_by_rank(entries: List[WishRankingEntry], rank: int) -> Optional[WishRankingEntry]:
    for entry in entries:
        if entry.rank == rank:
            return entry
    return None


def find_wish_entry_by_role_id(entries: List[WishRankingEntry], role_id: str) -> Optional[WishRankingEntry]:
    for entry in entries:
        if entry.role_id == role_id:
            return entry
    return None


def find_wish_line_entry(snapshot: WishRankingSnapshot, rank: int) -> Optional[WishRankingEntry]:
    entry = find_wish_entry_by_rank(snapshot.topN, rank)
    if entry:
        return entry
    return find_wish_entry_by_rank(snapshot.ladder, rank)


async def parse_wish_query_params(ctx: SekaiHandlerContext, args: str) -> Tuple[str, Union[str, int, List[int]]]:
    ats = ctx.get_at_qids()
    if ats:
        uid = get_player_bind_id(ctx, ats[0], check_bind=False)
        assert_and_reply(uid, "@的用户未绑定国服游戏ID")
        return "uid", uid

    args = args.strip()
    if not args:
        uid = get_player_bind_id(ctx, check_bind=False)
        if not uid:
            raise NoReplyException()
        return "self", uid

    ranks = []
    for seg in args.split():
        if not seg:
            continue
        if "-" in seg:
            start, end = seg.split("-", 1)
            start, end = get_rank_from_text(start), get_rank_from_text(end)
            assert_and_reply(start <= end, "查询排名范围错误: 起始排名大于结束排名")
            assert_and_reply(start >= 1 and end <= WISH_QUERY_MAX_RANK, f"心愿榜个人查询仅支持前{WISH_QUERY_MAX_RANK}名")
            assert_and_reply(end - start + 1 <= WISH_QUERY_MAX_RANK_COUNT, f"最多同时查询{WISH_QUERY_MAX_RANK_COUNT}个排名")
            for rank in range(start, end + 1):
                ranks.append(rank)
        elif is_rank_text(seg):
            rank = get_rank_from_text(seg)
            assert_and_reply(1 <= rank <= WISH_QUERY_MAX_RANK, f"心愿榜个人查询仅支持前{WISH_QUERY_MAX_RANK}名")
            ranks.append(rank)

    ranks = sorted(set(ranks))
    assert_and_reply(len(ranks) <= WISH_QUERY_MAX_RANK_COUNT, f"最多同时查询{WISH_QUERY_MAX_RANK_COUNT}个排名")
    if len(ranks) > 1:
        return "ranks", ranks
    if len(ranks) == 1:
        return "rank", ranks[0]

    raise ReplyException(f"""
查询方式:
1. 查询自己: {ctx.original_trigger_cmd}
2. 查询@用户: {ctx.original_trigger_cmd} @用户
3. 查询排名: {ctx.original_trigger_cmd} 100
4. 查询多个排名: {ctx.original_trigger_cmd} 1 10 20
""".strip())


def get_wish_query_text(qtype: str, qval: Union[str, int, List[int]]) -> str:
    if qtype == "self":
        return "你的心愿榜查询结果"
    if qtype == "uid":
        return "指定用户的心愿榜查询结果"
    if qtype == "rank":
        return f"排名 T{get_board_rank_str(qval)}"
    if qtype == "ranks":
        return "多个排名查询结果"
    return "心愿榜查询结果"


def get_remaining_time_text(end_at: Optional[datetime]) -> str:
    if not end_at:
        return "-"
    delta = end_at - get_wish_now_naive()
    if delta.total_seconds() <= 0:
        return "已结束"
    return get_readable_timedelta(delta)


def get_relative_time_text(dt: datetime) -> str:
    delta = get_wish_now_naive() - dt
    if delta.total_seconds() <= 0:
        return "刚刚"
    return get_readable_timedelta(delta) + "前"


def format_wish_event_title(event: Optional[dict]) -> str:
    if not event:
        return "-"
    return f"【CN-{event['id']}】{truncate(event['name'], 24)}"


def get_wish_rt_text(snapshot: WishRankingSnapshot) -> str:
    return f"RT: {snapshot.fetched_at.strftime('%Y-%m-%d %H:%M:%S')} ({get_relative_time_text(snapshot.fetched_at)})"


def get_wish_update_text(snapshot: WishRankingSnapshot) -> str:
    return f"更新时间: {snapshot.fetched_at.strftime('%Y-%m-%d %H:%M:%S')} ({get_relative_time_text(snapshot.fetched_at)})"


def get_wish_period_title(snapshot: WishRankingSnapshot) -> str:
    period_info = snapshot.period_info
    if period_info and period_info.period_index:
        if period_info.period_total:
            return f"心愿榜第{period_info.period_index}/{period_info.period_total}期"
        return f"心愿榜第{period_info.period_index}期"
    return "心愿榜信息"


async def get_next_event_in_wish_period(ctx: SekaiHandlerContext, period_info: Optional[WishPeriodInfo]) -> Optional[dict]:
    now = get_wish_now_naive()
    wish_end = period_info.end_at if period_info else None
    events = sorted(await ctx.md.events.get(), key=lambda x: x["startAt"])
    for event in events:
        start_at = datetime.fromtimestamp(event["startAt"] / 1000)
        if start_at <= now:
            continue
        if wish_end and start_at > wish_end:
            continue
        return event
    return None


async def count_remaining_events_in_wish_period(ctx: SekaiHandlerContext, period_info: Optional[WishPeriodInfo]) -> Optional[int]:
    if not period_info:
        return None
    now = get_wish_now_naive()
    count = 0
    for event in await ctx.md.events.get():
        start_at = datetime.fromtimestamp(event["startAt"] / 1000)
        end_at = datetime.fromtimestamp(event["aggregateAt"] / 1000 + 1)
        if end_at < now:
            continue
        if end_at < period_info.start_at or start_at > period_info.end_at:
            continue
        count += 1
    return count


async def build_wish_header_info(snapshot: WishRankingSnapshot) -> WishHeaderInfo:
    ctx = SekaiHandlerContext.from_region(CN_WISH_REGION)
    current_event = await get_current_event(ctx, fallback="prev_first")
    current_banner = await get_event_banner_img(ctx, current_event) if current_event else None
    next_event = await get_next_event_in_wish_period(ctx, snapshot.period_info)
    next_banner = await get_event_banner_img(ctx, next_event) if next_event else None
    t500 = find_wish_line_entry(snapshot, 500)

    current_start_at = datetime.fromtimestamp(current_event["startAt"] / 1000) if current_event else None
    current_end_at = datetime.fromtimestamp(current_event["aggregateAt"] / 1000 + 1) if current_event else None
    next_start_at = datetime.fromtimestamp(next_event["startAt"] / 1000) if next_event else None
    current_title = "当前活动"
    if current_event:
        current_title = f"当前活动 【CN-{current_event['id']}】{truncate(current_event['name'], 24)}"

    return WishHeaderInfo(
        title=get_wish_period_title(snapshot),
        current_title=current_title,
        current_event=current_event,
        current_banner=current_banner,
        current_start_at=current_start_at,
        current_end_at=current_end_at,
        current_remaining_text=get_remaining_time_text(current_end_at),
        period_info=snapshot.period_info,
        period_remaining_text=get_remaining_time_text(snapshot.period_info.end_at if snapshot.period_info else None),
        remaining_event_count=await count_remaining_events_in_wish_period(ctx, snapshot.period_info),
        t500_score_text=get_board_score_str(t500.pt) if t500 else "-",
        latest_rt_text=get_wish_rt_text(snapshot),
        next_event=next_event,
        next_start_at=next_start_at,
        next_banner=next_banner,
    )


async def save_wish_snapshot_data(data: dict) -> Tuple[WishRankingSnapshot, bool]:
    activity_id = str(data.get("activity_id") or "").strip()
    process_id = str(data.get("process_id") or "").strip()
    period_info = WishPeriodInfo.from_api(data.get("period_info") or {})
    ladder = [WishRankingEntry.from_api("ladder", item) for item in data.get("ladder", []) if item]
    topN = [WishRankingEntry.from_api("topN", item) for item in data.get("topN", []) if item]

    assert activity_id, "心愿榜数据缺少 activity_id"
    assert process_id, "心愿榜数据缺少 process_id"
    assert topN, "心愿榜数据缺少 topN"

    content_hash = compute_wish_content_hash(activity_id, process_id, ladder, topN)
    conn = await get_wish_conn(create=True)

    cursor = await conn.execute("""
        SELECT id, content_hash, activity_id
        FROM snapshot
        ORDER BY fetched_at DESC, id DESC
        LIMIT 1
    """)
    latest_row = await cursor.fetchone()
    await cursor.close()
    if latest_row and latest_row[1] == content_hash and latest_row[2] == activity_id:
        snapshot = await query_wish_snapshot_by_id(latest_row[0], load_entries=True)
        assert snapshot, "心愿榜快照读取失败"
        if period_info:
            await conn.execute("""
                UPDATE snapshot
                SET period_index = ?, period_total = ?, period_start_at = ?, period_end_at = ?
                WHERE id = ?
            """, (
                period_info.period_index,
                period_info.period_total,
                int(period_info.start_at.timestamp()),
                int(period_info.end_at.timestamp()),
                snapshot.id,
            ))
            await conn.commit()
        snapshot.period_info = period_info
        return snapshot, False

    now = get_wish_now()
    fetched_at = int(now.timestamp())
    cursor = await conn.execute("""
        INSERT INTO snapshot (
            activity_id, process_id, fetched_at, total_count, content_hash,
            period_index, period_total, period_start_at, period_end_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    """, (
        activity_id,
        process_id,
        fetched_at,
        len(topN),
        content_hash,
        period_info.period_index if period_info else None,
        period_info.period_total if period_info else None,
        int(period_info.start_at.timestamp()) if period_info else None,
        int(period_info.end_at.timestamp()) if period_info else None,
    ))
    snapshot_id = cursor.lastrowid
    await cursor.close()

    params = []
    for entry in ladder + topN:
        entry.snapshot_id = snapshot_id
        params.append((
            snapshot_id,
            entry.list_type,
            entry.rank,
            entry.role_id,
            entry.role_name,
            entry.pt,
            entry.server_id,
        ))
    await conn.executemany("""
        INSERT INTO ranking (snapshot_id, list_type, rank, role_id, role_name, pt, server_id)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    """, params)
    await conn.commit()

    logger.info(f"已保存国服心愿榜快照 activity_id={activity_id} topN={len(topN)}")
    return WishRankingSnapshot(
        id=snapshot_id,
        activity_id=activity_id,
        process_id=process_id,
        fetched_at=from_wish_timestamp(fetched_at),
        total_count=len(topN),
        content_hash=content_hash,
        period_info=period_info,
        ladder=ladder,
        topN=topN,
    ), True


async def fetch_wish_ranking_data(ctx: SekaiHandlerContext) -> dict:
    assert ctx.region == CN_WISH_REGION, "心愿榜仅支持国服"
    url = get_gameapi_config(ctx).wish_ranking_api_url
    assert url, "未配置国服心愿榜API地址"
    data = await request_gameapi(url)
    assert isinstance(data, dict), "心愿榜数据格式错误"
    assert isinstance(data.get("ladder"), list), "心愿榜数据缺少 ladder"
    assert isinstance(data.get("topN"), list), "心愿榜数据缺少 topN"
    return data


async def sync_wish_snapshot() -> Tuple[WishRankingSnapshot, bool]:
    async with _wish_sync_lock:
        ctx = SekaiHandlerContext.from_region(CN_WISH_REGION)
        data = await fetch_wish_ranking_data(ctx)
        return await save_wish_snapshot_data(data)


async def get_latest_wish_snapshot_for_query() -> WishRankingSnapshot:
    try:
        snapshot, _ = await sync_wish_snapshot()
        return snapshot
    except Exception as e:
        logger.warning(f"同步国服心愿榜失败，尝试使用本地缓存: {e}")

    snapshot = await query_latest_wish_snapshot(load_entries=True)
    assert_and_reply(snapshot, "暂无国服心愿榜数据")
    return snapshot


def need_daily_wish_refresh(snapshot: Optional[WishRankingSnapshot], now: datetime = None) -> bool:
    now = now or get_wish_now_naive()
    refresh_time = get_today_refresh_time(now)
    if now < refresh_time:
        return False
    if snapshot is None:
        return True
    return snapshot.fetched_at < refresh_time


def build_wish_metric_box(text: str, style: TextStyle, width: int):
    return TextBox(text, style, overflow="clip").set_size((width, 40)).set_padding((14, 0)).set_bg(roundrect_bg(fill=(255, 255, 255, 210)))


def render_wish_period_title(snapshot: WishRankingSnapshot, width: int = 818):
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=BLACK)
    TextBox(get_wish_period_title(snapshot), title_style).set_bg(roundrect_bg(fill=(255, 255, 255, 160))).set_size((width, 54)).set_padding((18, 8)).set_content_align("l")


def render_wish_event_panel(
    panel_title: str,
    event: Optional[dict],
    banner: Optional[Image.Image],
    start_at: Optional[datetime],
    end_at: Optional[datetime],
    remaining_text: str,
    remaining_label: str,
    width: int,
):
    panel_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=BLACK)
    name_style = TextStyle(font=DEFAULT_BOLD_FONT, size=19, color=BLACK)
    item_style = TextStyle(font=DEFAULT_FONT, size=17, color=BLACK)

    with VSplit().set_content_align("lt").set_item_align("lt").set_sep(8).set_padding(14).set_bg(roundrect_bg(fill=(255, 255, 255, 118))).set_size((width, None)):
        TextBox(panel_title, panel_title_style)
        TextBox(format_wish_event_title(event), name_style)
        if banner:
            with Frame().set_size((width - 28, 152)).set_content_align("c").set_bg(roundrect_bg(fill=(255, 255, 255, 86))):
                ImageBox(banner, size=(320, None))
        TextBox(f"开始时间: {start_at.strftime('%Y-%m-%d %H:%M:%S') if start_at else '-'}", item_style)
        if end_at is not None:
            TextBox(f"结束时间: {end_at.strftime('%Y-%m-%d %H:%M:%S')}", item_style)
        TextBox(f"{remaining_label}: {remaining_text}", item_style)


def render_wish_overview_header(snapshot: WishRankingSnapshot, header: WishHeaderInfo):
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=BLACK)
    item_style = TextStyle(font=DEFAULT_FONT, size=18, color=BLACK)
    width = 818
    panel_width = 401
    period_start = header.period_info.start_at.strftime('%Y-%m-%d %H:%M:%S') if header.period_info else "-"
    period_end = header.period_info.end_at.strftime('%Y-%m-%d %H:%M:%S') if header.period_info else "-"
    next_end_at = datetime.fromtimestamp(header.next_event["aggregateAt"] / 1000 + 1) if header.next_event else None

    with VSplit().set_content_align("lt").set_item_align("lt").set_sep(14).set_padding(16).set_bg(roundrect_bg(fill=(255, 255, 255, 160))):
        TextBox(header.title, title_style).set_size((width, 40))
        if header.period_info:
            TextBox(f"开始时间: {period_start}", item_style)
            TextBox(f"结束时间: {period_end}", item_style)
            TextBox(f"距离结束: {header.period_remaining_text}", item_style)
        TextBox(f"心愿榜ID: {snapshot.activity_id}    500线: {header.t500_score_text}", item_style)
        if header.remaining_event_count is not None:
            TextBox(f"剩余活动数量: {header.remaining_event_count}", item_style)
        with HSplit().set_content_align("lt").set_item_align("lt").set_sep(16).set_padding(0):
            render_wish_event_panel(
                "当前活动",
                header.current_event,
                header.current_banner,
                header.current_start_at,
                header.current_end_at,
                header.current_remaining_text,
                "剩余时间",
                panel_width,
            )
            render_wish_event_panel(
                "下期活动",
                header.next_event,
                header.next_banner,
                header.next_start_at,
                next_end_at,
                get_remaining_time_text(header.next_start_at) if header.next_start_at else "-",
                "距离开始",
                panel_width,
            )


def build_wish_entry_stats(
    snapshot: WishRankingSnapshot,
    prev_snapshot: Optional[WishRankingSnapshot],
    entry: WishRankingEntry,
    by_role: bool,
) -> dict:
    t500 = find_wish_entry_by_rank(snapshot.ladder, 500)
    prev_entry = find_wish_entry_by_role_id(prev_snapshot.topN, entry.role_id) if prev_snapshot else None
    prev_same_rank = find_wish_line_entry(prev_snapshot, entry.rank) if prev_snapshot else None
    prev_t500 = find_wish_entry_by_rank(prev_snapshot.ladder, 500) if prev_snapshot else None

    gap = entry.pt - t500.pt if t500 else None
    entry_delta = entry.pt - prev_entry.pt if prev_entry else None
    rank_delta = entry.pt - prev_same_rank.pt if prev_same_rank else None
    t500_delta = t500.pt - prev_t500.pt if t500 and prev_t500 else None

    return {
        "name": truncate(get_wish_entry_name(entry), 18),
        "rank": format_wish_rank(entry.rank),
        "score": get_board_score_str(entry.pt),
        "t500_score": get_board_score_str(t500.pt) if t500 else "-",
        "gap": format_wish_delta(gap),
        "speed": format_wish_delta(entry_delta if by_role else rank_delta),
        "speed_label": "个人日速" if by_role else "该名次日速",
        "t500_speed": format_wish_delta(t500_delta),
        "rt": snapshot.fetched_at.strftime("%H:%M:%S"),
        "rt_relative": get_relative_time_text(snapshot.fetched_at),
    }


def render_wish_single_query_card(
    snapshot: WishRankingSnapshot,
    prev_snapshot: Optional[WishRankingSnapshot],
    entry: WishRankingEntry,
    query_text: str,
    by_role: bool,
):
    stats = build_wish_entry_stats(snapshot, prev_snapshot, entry, by_role)
    name_style = TextStyle(font=DEFAULT_BOLD_FONT, size=30, color=BLACK)
    value_style = TextStyle(font=DEFAULT_FONT, size=20, color=BLACK)
    sub_style = TextStyle(font=DEFAULT_FONT, size=16, color=(70, 70, 70))
    card_width = 580

    with VSplit().set_content_align("lt").set_item_align("lt").set_sep(12).set_padding(16).set_bg(roundrect_bg(fill=(255, 255, 255, 172))).set_size((card_width, None)):
        TextBox(f"{stats['rank']}  {stats['name']}", name_style, overflow="clip").set_bg(roundrect_bg(fill=(255, 255, 255, 210))).set_size((card_width - 32, 56)).set_padding((18, 8)).set_content_align("l")
        with VSplit().set_content_align("lt").set_item_align("lt").set_sep(10).set_padding((18, 14)).set_bg(roundrect_bg(fill=(255, 255, 255, 118))).set_size((card_width - 32, None)):
            TextBox(f"活动积分: {stats['score']}", value_style).set_padding((0, 4))
            TextBox(f"500线: {stats['t500_score']}    与500线差值: {stats['gap']}", value_style).set_padding((0, 4))
            TextBox(f"{stats['speed_label']}: {stats['speed']}    500线日速: {stats['t500_speed']}", value_style).set_padding((0, 4))
            TextBox(get_wish_update_text(snapshot), sub_style).set_padding((0, 4))


def render_wish_table(
    snapshot: WishRankingSnapshot,
    prev_snapshot: Optional[WishRankingSnapshot],
    entries: List[WishRankingEntry],
    title: str,
    by_role: bool,
    include_name: bool,
):
    table_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=WHITE)
    table_item_style = TextStyle(font=DEFAULT_FONT, size=20, color=BLACK)
    sub_style = TextStyle(font=DEFAULT_FONT, size=16, color=(70, 70, 70))
    header_bg = FillBg((172, 126, 214, 250))
    row_bg1 = FillBg((255, 255, 255, 205))
    row_bg2 = FillBg((245, 238, 252, 205))

    stats_list = [build_wish_entry_stats(snapshot, prev_snapshot, entry, by_role) for entry in entries]

    with VSplit().set_content_align("lt").set_item_align("lt").set_sep(10).set_padding(16).set_bg(roundrect_bg(fill=(255, 255, 255, 172))):
        TextBox(get_wish_update_text(snapshot), sub_style)
        with HSplit().set_content_align("c").set_item_align("c").set_sep(4).set_padding(0):
            TextBox("排名", table_title_style).set_bg(header_bg).set_size((100, 42)).set_content_align("c")
            if include_name:
                TextBox("昵称", table_title_style).set_bg(header_bg).set_size((170, 42)).set_content_align("c")
            TextBox("活动积分", table_title_style).set_bg(header_bg).set_size((220, 42)).set_content_align("c")
            TextBox("500差值", table_title_style).set_bg(header_bg).set_size((170, 42)).set_content_align("c")
            TextBox("日速", table_title_style).set_bg(header_bg).set_size((142, 42)).set_content_align("c")

        for i, stats in enumerate(stats_list):
            bg = row_bg1 if i % 2 == 0 else row_bg2
            with HSplit().set_content_align("c").set_item_align("c").set_sep(4).set_padding(0):
                TextBox(stats["rank"], table_item_style, overflow="clip").set_bg(bg).set_size((100, 46)).set_content_align("r").set_padding((12, 0))
                if include_name:
                    TextBox(stats["name"], table_item_style, overflow="clip").set_bg(bg).set_size((170, 46)).set_content_align("l").set_padding((12, 0))
                TextBox(stats["score"], table_item_style, overflow="clip").set_bg(bg).set_size((220, 46)).set_content_align("r").set_padding((12, 0))
                TextBox(stats["gap"], table_item_style, overflow="clip").set_bg(bg).set_size((170, 46)).set_content_align("r").set_padding((12, 0))
                TextBox(stats["speed"], table_item_style, overflow="clip").set_bg(bg).set_size((142, 46)).set_content_align("r").set_padding((12, 0))


def render_wish_line_table(
    snapshot: WishRankingSnapshot,
    prev_snapshot: Optional[WishRankingSnapshot],
    entries: List[WishRankingEntry],
):
    table_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=WHITE)
    table_item_style = TextStyle(font=DEFAULT_FONT, size=20, color=BLACK)
    sub_style = TextStyle(font=DEFAULT_FONT, size=16, color=(70, 70, 70))
    header_bg = FillBg((172, 126, 214, 250))
    row_bg1 = FillBg((255, 255, 255, 205))
    row_bg2 = FillBg((245, 238, 252, 205))

    with VSplit().set_content_align("lt").set_item_align("lt").set_sep(10).set_padding(16).set_bg(roundrect_bg(fill=(255, 255, 255, 172))):
        TextBox(get_wish_update_text(snapshot), sub_style)
        with HSplit().set_content_align("c").set_item_align("c").set_sep(4).set_padding(0):
            TextBox("排名", table_title_style).set_bg(header_bg).set_size((150, 42)).set_content_align("c")
            TextBox("活动积分", table_title_style).set_bg(header_bg).set_size((402, 42)).set_content_align("c")
            TextBox("日速", table_title_style).set_bg(header_bg).set_size((258, 42)).set_content_align("c")

        for i, entry in enumerate(entries):
            prev_entry = find_wish_line_entry(prev_snapshot, entry.rank) if prev_snapshot else None
            delta = entry.pt - prev_entry.pt if prev_entry else None
            bg = row_bg1 if i % 2 == 0 else row_bg2
            with HSplit().set_content_align("c").set_item_align("c").set_sep(4).set_padding(0):
                TextBox(format_wish_rank(entry.rank), table_item_style, overflow="clip").set_bg(bg).set_size((150, 46)).set_content_align("r").set_padding((12, 0))
                TextBox(get_board_score_str(entry.pt), table_item_style, overflow="clip").set_bg(bg).set_size((402, 46)).set_content_align("r").set_padding((12, 0))
                TextBox(format_wish_delta(delta), table_item_style, overflow="clip").set_bg(bg).set_size((258, 46)).set_content_align("r").set_padding((12, 0))


async def compose_wishsk_image(
    snapshot: WishRankingSnapshot,
    prev_snapshot: Optional[WishRankingSnapshot],
    entries: List[WishRankingEntry],
    query_text: str,
    by_role: bool,
) -> Image.Image:
    header = await build_wish_header_info(snapshot)

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align("lt").set_item_align("lt").set_sep(12):
            if by_role and len(entries) == 1:
                render_wish_period_title(snapshot, width=580)
                render_wish_single_query_card(snapshot, prev_snapshot, entries[0], query_text, by_role)
            else:
                render_wish_overview_header(snapshot, header)
                render_wish_table(snapshot, prev_snapshot, entries, query_text, by_role, include_name=True)

    add_watermark(canvas)
    return await canvas.get_img()


async def compose_wishskl_image(snapshot: WishRankingSnapshot, prev_snapshot: Optional[WishRankingSnapshot]) -> Image.Image:
    header = await build_wish_header_info(snapshot)

    line_entries = []
    for rank in WISH_LINE_RANKS:
        entry = find_wish_line_entry(snapshot, rank)
        if entry:
            line_entries.append(entry)

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align("lt").set_item_align("lt").set_sep(12):
            render_wish_overview_header(snapshot, header)
            if line_entries:
                render_wish_line_table(snapshot, prev_snapshot, line_entries)
            else:
                TextBox("暂无心愿榜榜线数据", TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=BLACK)).set_padding(32)

    add_watermark(canvas)
    return await canvas.get_img()


pjsk_wishsk = SekaiCmdHandler([
    "/wishsk",
], regions=["cn"])
pjsk_wishsk.check_cdrate(cd).check_wblist(gbl)
@pjsk_wishsk.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    qtype, qval = await parse_wish_query_params(ctx, args)

    snapshot = await get_latest_wish_snapshot_for_query()
    prev_snapshot = await query_prev_wish_snapshot(snapshot.activity_id)

    entries: List[WishRankingEntry] = []
    if qtype in ("self", "uid"):
        role_id = get_wish_role_id(qval)
        entry = find_wish_entry_by_role_id(snapshot.topN, role_id)
        if not entry:
            t500 = find_wish_entry_by_rank(snapshot.ladder, 500)
            t500_text = get_board_score_str(t500.pt) if t500 else "-"
            return await ctx.asend_reply_msg(f"""
当前未进入国服心愿榜前500
当前500线: {t500_text}
更新时间: {snapshot.fetched_at.strftime('%Y-%m-%d %H:%M:%S')}
""".strip())
        entries = [entry]
    elif qtype == "rank":
        entry = find_wish_entry_by_rank(snapshot.topN, qval)
        assert_and_reply(entry, f"找不到排名 T{get_board_rank_str(qval)} 的心愿榜数据")
        entries = [entry]
    elif qtype == "ranks":
        entries = [find_wish_entry_by_rank(snapshot.topN, rank) for rank in qval]
        entries = [entry for entry in entries if entry]
        assert_and_reply(entries, "找不到指定排名的心愿榜数据")
    else:
        raise ReplyException("不支持的心愿榜查询方式")

    return await ctx.asend_msg(await get_image_cq(
        await compose_wishsk_image(
            snapshot,
            prev_snapshot,
            entries,
            get_wish_query_text(qtype, qval),
            by_role=qtype in ("self", "uid"),
        ),
        low_quality=True,
    ))


pjsk_wishskl = SekaiCmdHandler([
    "/wishsk线",
], regions=["cn"], parse_uid_arg=False)
pjsk_wishskl.check_cdrate(cd).check_wblist(gbl)
@pjsk_wishskl.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    assert_and_reply(not args, f"使用方式: {ctx.original_trigger_cmd}")

    snapshot = await get_latest_wish_snapshot_for_query()
    prev_snapshot = await query_prev_wish_snapshot(snapshot.activity_id)
    return await ctx.asend_msg(await get_image_cq(
        await compose_wishskl_image(snapshot, prev_snapshot),
        low_quality=True,
    ))


@repeat_with_interval(WISH_CHECK_INTERVAL_CFG, "同步国服心愿榜数据", logger, error_limit=1)
async def sync_wish_snapshot_task():
    latest_snapshot = await query_latest_wish_snapshot(load_entries=False)
    if not need_daily_wish_refresh(latest_snapshot):
        return

    snapshot, created = await sync_wish_snapshot()
    if created:
        logger.info(f"国服心愿榜已刷新 activity_id={snapshot.activity_id}")
    else:
        logger.info("国服心愿榜仍未刷新，等待下次重试")

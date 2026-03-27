from ...utils import *
from ..common import *
from ..handler import *
from ..asset import *
from ..draw import *
from .resbox import get_res_box_info, get_res_icon
from .profile import (
    get_detailed_profile,
    get_detailed_profile_card,
    get_detailed_profile_card_filter,
    get_player_avatar_info_by_detailed_profile,
    get_user_data_mode,
    process_hide_uid,
    ts_to_dt,
)
import glob
from collections import defaultdict

@dataclass
class AreaItemFilter:
    unit: str = None        # 某个团的世界里面的所有道具
    cid: int = None         # 某个角色的所有道具
    attr: str = None        # 某个属性的所有道具
    tree: bool = None       # 所有树
    flower: bool = None     # 所有花

FLOWER_AREA_ID = 13
TREE_AREA_ID = 11
UNIT_SEKAI_AREA_IDS = {
    "light_sound": 5,
    "idol": 7,
    "street": 8,
    "theme_park": 9,
    "school_refusal": 10,
}


CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS = True


CHARACTER_MISSION_SECTION_DEFS = [
    {
        "key": "collect",
        "title": "收集",
        "color": (255, 118, 162, 255),
        "header_fill": (255, 220, 232, 230),
        "items": [
            ("collect_member", "成员", 14),
            ("collect_costume_3d", "服装", 3),
            ("collect_another_vocal", "Another Vocal", 8),
            ("read_area_talk", "区域对话", 5),
            ("collect_stamp", "表情", 4),
            ("collect_character_archive_voice", "台词", 19),
        ],
    },
    {
        "key": "play_story",
        "title": "游玩与剧情",
        "color": (95, 153, 255, 255),
        "header_fill": (221, 233, 255, 230),
        "items": [
            ("play_live", "队长游玩", 1),
            ("waiting_room", "送到休息室", 2),
            ("read_card_episode_first", "卡牌剧情上篇", 6),
            ("read_card_episode_second", "卡牌剧情下篇", 7),
        ],
    },
    {
        "key": "education",
        "title": "养成",
        "color": (255, 170, 74, 255),
        "header_fill": (255, 238, 211, 230),
        "items": [
            ("area_item_level_up_character", "角色区域道具", 9),
            ("area_item_level_up_unit", "组合区域道具", 11),
            ("area_item_level_up_reality_world", "类型区域道具", 13),
            ("skill_level_up_rare", "技能等级提升★4・生日・纪念日", 15),
            ("skill_level_up_standard", "技能等级提升★1~★3", 16),
            ("master_rank_up_rare", "专家等级提升★4・生日・纪念日", 17),
            ("master_rank_up_standard", "专家等级提升★1~★3", 18),
        ],
    },
    {
        "key": "mysekai",
        "title": "Mysekai",
        "color": (89, 201, 153, 255),
        "header_fill": (222, 245, 235, 230),
        "items": [
            ("read_mysekai_fixture_unique_character_talk", "我的「世界」", 22),
            ("collect_mysekai_fixture", "附带角色标签的家具", 20),
            ("collect_mysekai_canvas", "装饰画", 21),
        ],
    },
    {
        "key": "ex",
        "title": "EX",
        "color": (170, 108, 255, 255),
        "header_fill": (236, 225, 255, 230),
        "note": "普通队长游玩达到40000后解锁",
        "items": [
            ("play_live_ex", "队长游玩 EX", 101),
            ("waiting_room_ex", "送到休息室 EX", 102),
        ],
    },
]


CHARACTER_MISSION_MASTER_SECTION_DEFS = [
    {
        "key": "collect",
        "title": "收集",
        "color": (255, 118, 162, 255),
        "header_fill": (255, 220, 232, 230),
        "items": [
            ("成员", [(14, "")]),
            ("服装", [(3, "")]),
            ("Another Vocal", [(8, "")]),
            ("区域对话", [(5, "")]),
            ("表情", [(4, "")]),
            ("台词", [(19, "")]),
        ],
    },
    {
        "key": "play_story",
        "title": "游玩与剧情",
        "color": (95, 153, 255, 255),
        "header_fill": (221, 233, 255, 230),
        "note": "队长游玩与送到休息室已合并 EX 档位，EX 行会单独标记。",
        "items": [
            ("队长游玩", [(1, ""), (101, "EX")]),
            ("送到休息室", [(2, ""), (102, "EX")]),
            ("卡牌剧情上篇", [(6, "")]),
            ("卡牌剧情下篇", [(7, "")]),
        ],
    },
    {
        "key": "education",
        "title": "养成",
        "color": (255, 170, 74, 255),
        "header_fill": (255, 238, 211, 230),
        "items": [
            ("角色区域道具", [(9, "")]),
            ("组合区域道具", [(11, "")]),
            ("类型区域道具", [(13, "")]),
            ("技能等级提升★4・生日・纪念日", [(15, "")]),
            ("技能等级提升★1~★3", [(16, "")]),
            ("专家等级提升★4・生日・纪念日", [(17, "")]),
            ("专家等级提升★1~★3", [(18, "")]),
        ],
    },
    {
        "key": "mysekai",
        "title": "Mysekai",
        "color": (89, 201, 153, 255),
        "header_fill": (222, 245, 235, 230),
        "items": [
            ("我的「世界」", [(22, "")]),
            ("附带角色标签的家具", [(20, "")]),
            ("装饰画", [(21, "")]),
        ],
    },
]

CHARACTER_MISSION_MASTER_OVERVIEW_LOCKS = {}


def require_profile_field(profile: dict, key: str):
    assert_and_reply(key in profile, f"你的Suite数据来源没有提供{key}数据")
    return profile[key]


async def get_character_mission_fullbody_image(ctx: SekaiHandlerContext, cid: int) -> Image.Image:
    rel_path = f"chara_icon/{cid}.png"
    full_path = pjoin(DEFAULT_STATIC_IMAGE_DIR, rel_path)
    assert_and_reply(osp.exists(full_path), f"未找到角色 {cid} 的小人头像资源")
    return ctx.static_imgs.get(rel_path)


def get_character_mission_icon_image(ctx: SekaiHandlerContext, cid: int) -> Image.Image:
    rel_path = f"chara_icon/{cid}.png"
    full_path = pjoin(DEFAULT_STATIC_IMAGE_DIR, rel_path)
    assert_and_reply(osp.exists(full_path), f"未找到角色 {cid} 的小人头像资源")
    return ctx.static_imgs.get(rel_path)


def build_character_mission_progress_bar(width: int, progress: float, color: Color) -> Frame:
    progress = max(min(progress, 1), 0)
    total_h, border = 16, 2
    fill_w = int((width - border * 2) * progress)
    bar_bg = (*color[:3], 55)
    tick_color = (*color[:3], 120)
    with Frame().set_size((width, total_h)).set_content_align('lt') as ret:
        Spacer(width, total_h).set_bg(RoundRectBg(fill=bar_bg, radius=total_h // 2))
        if fill_w > 0:
            Spacer(fill_w, total_h - border * 2).set_bg(
                RoundRectBg(fill=color, radius=(total_h - border) // 2)
            ).set_offset((border, border))
        for idx in range(1, 5):
            tick_x = int(width * idx / 5)
            Spacer(2, total_h - 8).set_bg(FillBg(tick_color)).set_offset((tick_x - 1, 4))
    return ret


def soften_color(color: Color, ratio: float = 0.8, alpha: int | None = None) -> Color:
    r, g, b = color[:3]
    a = color[3] if len(color) >= 4 else 255
    if alpha is not None:
        a = alpha
    return (
        int(r + (255 - r) * ratio),
        int(g + (255 - g) * ratio),
        int(b + (255 - b) * ratio),
        a,
    )


def mix_color(color_a: Color, color_b: Color, ratio: float = 0.5, alpha: int | None = None) -> Color:
    ratio = max(0, min(1, ratio))
    r = int(color_a[0] * (1 - ratio) + color_b[0] * ratio)
    g = int(color_a[1] * (1 - ratio) + color_b[1] * ratio)
    b = int(color_a[2] * (1 - ratio) + color_b[2] * ratio)
    a = alpha if alpha is not None else (color_b[3] if len(color_b) >= 4 else 255)
    return (r, g, b, a)


def build_character_mission_next_badge(delta: int, color: Color) -> Frame:
    badge_fill = soften_color(color, ratio=0.18, alpha=255)
    with HSplit().set_content_align('c').set_item_align('c').set_sep(0).set_padding((12, 4)).set_bg(
        RoundRectBg(fill=badge_fill, radius=17)
    ) as ret:
        TextBox(f"+{delta}", TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=WHITE))
    return ret


def get_character_mission_reward_value(row: dict, group_id: int | None = None) -> int:
    exp = int(row.get('exp', 0) or 0)
    if exp > 0:
        return exp
    quantity = int(row.get('quantity', 0) or 0)
    if quantity > 0 and group_id is not None:
        logger.info(f"character mission: parameter group {group_id} reward exp=0, use quantity={quantity}")
    return quantity


def build_character_mission_overall_dots(ctx: SekaiHandlerContext, filled: int) -> Frame:
    filled = max(0, min(10, filled))
    active = ctx.static_imgs.get("exp_active.png")
    inactive = ctx.static_imgs.get("exp_noactive.png")
    with VSplit().set_content_align('l').set_item_align('l').set_sep(6) as ret:
        for row_len, start_idx in [(6, 0), (4, 6)]:
            with HSplit().set_content_align('l').set_item_align('c').set_sep(4):
                for idx in range(start_idx, start_idx + row_len):
                    ImageBox(active if idx < filled else inactive, size=(20, 20), use_alphablend=True)
    return ret


def build_character_mission_next_cell(item: dict, color: Color, width: int) -> Frame:
    value_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(115, 82, 78))
    next_text = str(item['next'])
    value_w = max(56, get_str_display_length(next_text) * 18 + 4)
    with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_w(width) as ret:
        TextBox(next_text, value_style).set_w(value_w).set_content_align('r')
        if not item['is_max'] and item['next_reward'] > 0:
            build_character_mission_next_badge(item['next_reward'], color)
    return ret


async def get_character_mission_theme_color(ctx: SekaiHandlerContext, cid: int) -> Color:
    unit_info = await ctx.md.game_character_units.find_by('gameCharacterId', cid)
    assert_and_reply(unit_info, f"角色 {cid} 缺少 gameCharacterUnits 数据")
    return color_code_to_rgb(unit_info['colorCode'])


async def get_character_mission_rank_exp_dots(
    ctx: SekaiHandlerContext,
    chara_rank: int,
    total_exp: int,
) -> int:
    level_rows = await ctx.md.levels.find_by('levelType', 'character', mode='all')
    assert_and_reply(level_rows, "MasterData 缺少 character levels 数据")
    current_level_row = find_by(level_rows, 'level', chara_rank)
    assert_and_reply(current_level_row, f"MasterData 缺少角色等级 {chara_rank} 的 levels 数据")
    next_level_row = find_by(level_rows, 'level', chara_rank + 1)
    current_level_exp = int(current_level_row['totalExp'])
    if next_level_row is None:
        return 10
    next_level_exp = int(next_level_row['totalExp'])
    level_progress = max(0, total_exp - current_level_exp)
    level_total = max(next_level_exp - current_level_exp, 1)
    if level_total <= 10:
        return max(0, min(10, level_progress))
    return max(0, min(10, int(round(level_progress / level_total * 10))))


def normalize_character_mission_challenge_stage_rows(rows: list) -> list[dict]:
    if not rows:
        return []
    if isinstance(rows[0], dict):
        return rows
    assert_and_reply(isinstance(rows[0], list), "userChallengeLiveSoloStages 数据格式不支持")
    logger.info("角色任务: 检测到 userChallengeLiveSoloStages 为压缩数组，按 suite-update 结构展开")
    normalized = []
    for row in rows:
        assert_and_reply(len(row) >= 6, "userChallengeLiveSoloStages 压缩数组字段数量不足")
        normalized.append({
            "challengeLiveStageType": row[0],
            "characterId": row[1],
            "challengeLiveStageId": row[2],
            "rank": row[3],
            "challengeLiveStageStatus": row[4],
            "point": row[5],
        })
    return normalized


async def build_character_mission_profile_panel(
    ctx: SekaiHandlerContext,
    profile: dict,
    err_msg: str,
    theme: Color | None = None,
) -> Frame:
    avatar_info = await get_player_avatar_info_by_detailed_profile(ctx, profile)
    game_data = require_profile_field(profile, 'userGamedata')
    upload_time = ts_to_dt(require_profile_field(profile, 'upload_time'))
    source = profile.get('source', '?')
    if local_source := profile.get('local_source'):
        source += f"({local_source})"
    mode = get_user_data_mode(ctx, ctx.user_id)
    update_time_text = upload_time.strftime('%m-%d %H:%M:%S') + f" ({get_readable_datetime(upload_time, show_original_time=False)})"
    user_id = process_hide_uid(ctx, game_data['userId'], keep=6)

    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=BLACK)
    info_style = TextStyle(font=DEFAULT_FONT, size=16, color=(36, 36, 36))
    err_style = TextStyle(font=DEFAULT_FONT, size=16, color=RED)
    panel_fill = (
        soften_color(theme, ratio=0.78, alpha=220)
        if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS and theme
        else (255, 255, 255, 205)
    )
    avatar_fill = (
        soften_color(theme, ratio=0.9, alpha=155)
        if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS and theme
        else (255, 255, 255, 120)
    )

    with HSplit().set_content_align('lt').set_item_align('c').set_sep(14).set_padding(16).set_size((610, 166)).set_bg(
        roundrect_bg(fill=panel_fill)
    ) as ret:
        ImageBox(avatar_info.img, size=(84, 84), image_size_mode='fill', use_alphablend=True, shadow=True).set_bg(
            RoundRectBg(fill=avatar_fill, radius=14)
        )
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(4).set_w(480):
            TextBox(truncate(game_data['name'], 32), title_style).set_w(470).set_overflow('clip')
            TextBox(f"{ctx.region.upper()}: {user_id} Suite数据", info_style)
            TextBox(f"更新时间: {update_time_text}", info_style)
            TextBox(f"数据来源: {source}  获取模式: {mode}", info_style).set_w(470).set_overflow('clip')
            if err_msg:
                TextBox(f"获取数据失败: {err_msg}", err_style).set_w(470).set_overflow('clip')
    return ret


async def get_character_mission_overview_data(
    ctx: SekaiHandlerContext,
    profile: dict,
    cid: int | None = None,
) -> dict:
    ucms = require_profile_field(profile, 'userCharacterMissionV2s')
    ucm_ss = require_profile_field(profile, 'userCharacterMissionV2Statuses')
    user_characters = require_profile_field(profile, 'userCharacters')
    challenge_results = require_profile_field(profile, 'userChallengeLiveSoloResults')
    challenge_stages = normalize_character_mission_challenge_stage_rows(
        require_profile_field(profile, 'userChallengeLiveSoloStages')
    )

    if cid is None:
        avatar_info = await get_player_avatar_info_by_detailed_profile(ctx, profile)
        cid = avatar_info.cid

    chara = await ctx.md.game_characters.find_by_id(cid)
    assert_and_reply(chara, f"character {cid} not found")

    mission_defs = {
        item['characterMissionType']: item
        for item in await ctx.md.character_mission_v2s.find_by('characterId', cid, mode='all')
    }

    parameter_groups: dict[int, list[dict]] = defaultdict(list)
    for item in await ctx.md.character_mission_v2_parameter_groups.get():
        parameter_groups[item['id']].append(item)
    for rows in parameter_groups.values():
        rows.sort(key=lambda x: x['requirement'])

    progress_map = {}
    for item in find_by(ucms, 'characterId', cid, mode='all'):
        progress_map[item['characterMissionType']] = item.get('progress', 0)

    status_map: dict[int, set[int]] = defaultdict(set)
    for item in find_by(ucm_ss, 'characterId', cid, mode='all'):
        group_id = item.get('parameterGroupId')
        seq = item.get('seq')
        if group_id is None or seq is None:
            continue
        status_map[group_id].add(seq)

    section_results = []
    all_items = []
    base_completed = 0
    base_total = 0

    for section in CHARACTER_MISSION_SECTION_DEFS:
        section_items = []
        for mission_type, title, group_id in section['items']:
            mission_def = mission_defs.get(mission_type)
            parameter_rows = parameter_groups.get(group_id, [])
            assert_and_reply(mission_def, f"MasterData missing character {cid} mission definition {mission_type}")
            assert_and_reply(parameter_rows, f"MasterData missing character mission parameter group {group_id}")
            progress = int(progress_map.get(mission_type, 0))
            completed_from_status = len(status_map.get(group_id, set()))
            completed_from_progress = sum(progress >= row['requirement'] for row in parameter_rows)
            completed = max(completed_from_status, completed_from_progress)
            total = len(parameter_rows)
            max_value = parameter_rows[-1]['requirement'] if parameter_rows else 0
            next_row = parameter_rows[min(completed, total - 1)] if total else None
            next_value = next_row['requirement'] if next_row else 0
            next_reward = get_character_mission_reward_value(next_row, group_id=group_id) if next_row else 0
            stamp_current = sum(get_character_mission_reward_value(row) for row in parameter_rows[:min(completed, total)])
            stamp_total = sum(get_character_mission_reward_value(row) for row in parameter_rows)
            item_data = {
                'mission_type': mission_type,
                'title': title,
                'group_id': group_id,
                'sentence': mission_def.get('sentence', '') if mission_def else '',
                'progress': progress,
                'completed': completed,
                'total': total,
                'stamp_current': stamp_current,
                'stamp_total': stamp_total,
                'next': next_value,
                'next_reward': next_reward if total and completed < total else 0,
                'max': max_value,
                'max_text': f"累计{max_value}" if section['key'] == 'ex' else str(max_value),
                'ratio': (progress / max_value) if max_value else 0,
                'is_max': bool(total and completed >= total),
            }
            section_items.append(item_data)
            all_items.append(item_data)
            if section['key'] != 'ex':
                base_completed += completed
                base_total += total
        section_results.append({**section, 'items': section_items})

    user_character = find_by(user_characters, 'characterId', cid)
    assert_and_reply(user_character, f"Suite missing character {cid} rank info")
    chara_rank = user_character.get('characterRank', 0)
    chara_total_exp = int(user_character.get('totalExp', 0))
    rank_exp_dots = await get_character_mission_rank_exp_dots(ctx, chara_rank, chara_total_exp)

    challenge_result = find_by(challenge_results, 'characterId', cid) or {}
    chara_stages = find_by(challenge_stages, 'characterId', cid, mode='all')
    stage_rank = max([item.get('rank', 0) for item in chara_stages], default=0) if chara_stages else 0
    current_stage_row = find_by(chara_stages, 'rank', stage_rank) if stage_rank else None
    high_score = challenge_result.get('highScore', 0)

    stage_master = await RegionMasterDataWrapper(ctx.region, "challengeLiveStages").find_by('characterId', cid, mode='all')
    stage_master = sorted(stage_master, key=lambda x: x['rank']) if stage_master else []
    current_stage_master = find_by(stage_master, 'rank', stage_rank) if stage_rank else None
    assert_and_reply(stage_rank == 0 or current_stage_master, f"MasterData missing character {cid} challenge stage {stage_rank}")
    next_stage_point = current_stage_master.get('nextStageChallengePoint', 0) if current_stage_master else 0
    challenge_point = challenge_result.get('challengePoint')
    stage_point = current_stage_row.get('point', 0) if current_stage_row else 0
    if challenge_result and challenge_point is None and stage_point:
        logger.info(f"character mission: character {cid} missing challengePoint, use userChallengeLiveSoloStages.point={stage_point}")
        challenge_point = stage_point
    elif challenge_result and challenge_point is None:
        logger.info(f"character mission: character {cid} missing challengePoint, summary only shows next-stage target")
    challenge_remain = max(next_stage_point - challenge_point, 0) if next_stage_point and challenge_point is not None else None
    challenge_ratio = (challenge_point / next_stage_point) if next_stage_point and challenge_point is not None else 0
    challenge_ratio = max(0, min(1, challenge_ratio))

    overall_ratio = (base_completed / base_total) if base_total else 0

    return {
        'cid': cid,
        'chara': chara,
        'chara_rank': chara_rank,
        'chara_total_exp': chara_total_exp,
        'section_results': section_results,
        'all_items': all_items,
        'base_completed': base_completed,
        'base_total': base_total,
        'overall_ratio': overall_ratio,
        'rank_exp_dots': rank_exp_dots,
        'stage_rank': stage_rank,
        'high_score': high_score,
        'next_stage_point': next_stage_point,
        'challenge_remain': challenge_remain,
        'challenge_ratio': challenge_ratio,
    }


async def build_character_mission_summary_panel(
    ctx: SekaiHandlerContext,
    profile: dict,
    overview: dict,
) -> Frame:
    cid = overview['cid']
    chara = overview['chara']
    fullbody = await get_character_mission_fullbody_image(ctx, cid)
    chara_icon = get_character_mission_icon_image(ctx, cid)
    theme = await get_character_mission_theme_color(ctx, cid)
    panel_fill = soften_color(theme, ratio=0.72, alpha=232)
    white_card = (
        soften_color(theme, ratio=0.84, alpha=236)
        if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS
        else (255, 255, 255, 230)
    )
    title_fill = theme
    line_fill = soften_color(theme, ratio=0.15, alpha=255)
    name_color = theme
    en_name_color = soften_color(theme, ratio=0.28, alpha=255)
    info_title_color = soften_color(theme, ratio=0.35, alpha=255)
    stage_badge_fill = soften_color(theme, ratio=0.1, alpha=255)
    stage_bar_fill = (226, 230, 238, 255)
    rank_fill = (52, 193, 255, 255)

    cn_name = f"{chara.get('firstName', '')}{chara.get('givenName', '')}"
    en_name = f"{chara.get('firstNameEnglish', '')} {chara.get('givenNameEnglish', '')}".strip().upper()
    stage_remain_text = (
        f"还差{overview['challenge_remain']}积分"
        if overview['challenge_remain'] is not None
        else (f"下一档{overview['next_stage_point']}pt" if overview['next_stage_point'] else "未读取到挑战点")
    )

    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=WHITE)
    name_style = TextStyle(font=DEFAULT_BOLD_FONT, size=34, color=name_color)
    en_name_style = TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=en_name_color)
    small_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=info_title_color)
    value_style = TextStyle(font=DEFAULT_BOLD_FONT, size=36, color=theme)
    white_value_style = TextStyle(font=DEFAULT_BOLD_FONT, size=26, color=WHITE)
    rank_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(255, 255, 255, 235))
    stage_label_style = TextStyle(font=DEFAULT_BOLD_FONT, size=14, color=WHITE)

    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(14).set_padding(18).set_size((708, 348)).set_bg(
        roundrect_bg(fill=panel_fill)
    ) as ret:
        with HSplit().set_content_align('lt').set_item_align('lt').set_sep(14):
            with Frame().set_size((286, 160)).set_bg(roundrect_bg(
                fill=soften_color(theme, ratio=0.8, alpha=180) if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS else (255, 255, 255, 155)
            )).set_content_align('c'):
                ImageBox(fullbody, size=(156, 156), use_alphablend=True, shadow=True)

            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_padding((18, 12)).set_size((372, 160)).set_bg(
                roundrect_bg(fill=white_card)
            ):
                with HSplit().set_content_align('l').set_item_align('c').set_sep(10):
                    TextBox("印章任务", title_style).set_padding((16, 8)).set_bg(
                        RoundRectBg(fill=title_fill, radius=18)
                    )
                    Spacer(12, 12).set_bg(RoundRectBg(fill=(255, 168, 0, 255), radius=6))
                Spacer(336, 7).set_bg(RoundRectBg(fill=line_fill, radius=3))
                TextBox(cn_name, name_style).set_overflow('clip')
                TextBox(en_name, en_name_style).set_overflow('clip')

        with HSplit().set_content_align('lt').set_item_align('lt').set_sep(14):
            with HSplit().set_content_align('lt').set_item_align('c').set_sep(14).set_padding((16, 12)).set_size((286, 138)).set_bg(
                roundrect_bg(fill=white_card)
            ):
                with VSplit().set_content_align('c').set_item_align('c').set_sep(4).set_padding((12, 10)).set_w(94).set_bg(
                    roundrect_bg(fill=rank_fill)
                ):
                    ImageBox(chara_icon, size=(34, 34), use_alphablend=True)
                    TextBox("等级", rank_title_style)
                    TextBox(str(overview['chara_rank']), white_value_style)
                build_character_mission_overall_dots(ctx, overview['rank_exp_dots'])

            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(4).set_padding((16, 8)).set_size((372, 138)).set_bg(
                roundrect_bg(fill=white_card)
            ):
                TextBox("挑战等级", small_title_style)
                with HSplit().set_content_align('lt').set_item_align('c').set_sep(14):
                    ImageBox(chara_icon, size=(50, 50), use_alphablend=True)
                    with VSplit().set_content_align('c').set_item_align('c').set_sep(0).set_size((54, 54)).set_bg(
                        RoundRectBg(fill=stage_badge_fill, radius=27)
                    ):
                        TextBox("stage", stage_label_style)
                        TextBox(str(overview['stage_rank'] or '-'), white_value_style.replace(size=22))
                    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(2):
                        TextBox(stage_remain_text, small_title_style.replace(color=theme))
                        build_character_mission_progress_bar(154, overview['challenge_ratio'], theme)
                        TextBox("最高分", small_title_style)
                        TextBox(str(overview['high_score'] or '-'), value_style.replace(size=24))
    return ret


async def build_character_mission_section(section: dict) -> Frame:
    color = section['color']
    theme: Color | None = section.get('theme_color')
    if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS and theme:
        theme_fill = soften_color(theme, ratio=0.86, alpha=225)
        header_fill = mix_color(soften_color(theme, ratio=0.82, alpha=230), section['header_fill'], 0.42, alpha=230)
        section_fill = mix_color(theme_fill, color, 0.18, alpha=225)
        row_even_fill = mix_color(soften_color(theme, ratio=0.9, alpha=165), color, 0.14, alpha=165)
        row_odd_fill = mix_color(soften_color(theme, ratio=0.94, alpha=132), color, 0.1, alpha=132)
    else:
        header_fill = section['header_fill']
        section_fill = soften_color(color, ratio=0.9, alpha=225)
        row_even_fill = soften_color(color, ratio=0.93, alpha=165)
        row_odd_fill = soften_color(color, ratio=0.96, alpha=132)
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=WHITE)
    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=(72, 86, 118))
    text_style = TextStyle(font=DEFAULT_FONT, size=20, color=(101, 82, 94))
    bold_style = text_style.replace(font=DEFAULT_BOLD_FONT, color=(112, 84, 86))
    percent_style = text_style.replace(font=DEFAULT_BOLD_FONT, size=16, color=(*color[:3], 190))
    note_style = text_style.replace(size=18, color=(120, 120, 120))

    w1, w2, w3, w4, w5, w6 = 420, 150, 120, 176, 140, 232

    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(10).set_padding(18).set_bg(
        roundrect_bg(fill=section_fill)
    ) as ret:
        with HSplit().set_content_align('l').set_item_align('c').set_sep(10):
            TextBox(section['title'], title_style).set_padding((18, 8)).set_bg(RoundRectBg(fill=color, radius=18))
            if section.get('note'):
                TextBox(section['note'], note_style)

        with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(52).set_padding((10, 4)).set_bg(RoundRectBg(fill=header_fill, radius=14)):
            TextBox("任务项", header_style).set_w(w1).set_content_align('c')
            TextBox("完成度", header_style).set_w(w2).set_content_align('c')
            TextBox("当前值", header_style).set_w(w3).set_content_align('c')
            TextBox("下一档", header_style).set_w(w4).set_content_align('c')
            TextBox("MAX", header_style).set_w(w5).set_content_align('c')
            TextBox("进度", header_style).set_w(w6).set_content_align('c')

        for idx, item in enumerate(section['items']):
            row_fill = row_even_fill if idx % 2 == 0 else row_odd_fill
            with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(54).set_padding((10, 4)).set_bg(roundrect_bg(fill=row_fill)):
                with HSplit().set_content_align('l').set_item_align('c').set_sep(12).set_w(w1):
                    Spacer(8, 28).set_bg(RoundRectBg(fill=color, radius=4))
                    TextBox(item['title'], text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w1 - 20).set_overflow('clip')

                TextBox(f"{item['stamp_current']}/{item['stamp_total']}", bold_style).set_w(w2).set_content_align('c')
                TextBox(str(item['progress']), bold_style).set_w(w3).set_content_align('c')
                build_character_mission_next_cell(item, color, w4)
                TextBox(item['max_text'], bold_style).set_w(w5).set_content_align('c')

                with HSplit().set_content_align('l').set_item_align('c').set_sep(10).set_w(w6):
                    build_character_mission_progress_bar(150, item['ratio'], color)
                    TextBox(f"{item['ratio'] * 100:.1f}%", percent_style).set_w(72).set_content_align('r')
    return ret


async def compose_character_mission_image(
    ctx: SekaiHandlerContext,
    qid: int,
    cid: int | None = None,
) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx,
        qid,
        filter=get_detailed_profile_card_filter(
            'userCharacterMissionV2s',
            'userCharacterMissionV2Statuses',
            'userCharacters',
            'userChallengeLiveSoloResults',
            'userChallengeLiveSoloStages',
        ),
        raise_exc=True,
    )

    overview = await get_character_mission_overview_data(ctx, profile, cid)
    theme = await get_character_mission_theme_color(ctx, overview['cid'])

    note_lines = [
        "MAX 基于 MasterData，未必等于当前游戏端已实装的上限。",
        "角色等级与挑战等级读取自当前 Suite 抓包。"
    ]

    note_fill = (
        soften_color(theme, ratio=0.9, alpha=232)
        if CHARACTER_MISSION_USE_THEME_INFO_BACKGROUNDS
        else (244, 251, 255, 232)
    )

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                    await build_character_mission_profile_panel(ctx, profile, err_msg, theme)
                    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(10).set_padding(18).set_size((610, 166)).set_bg(
                        roundrect_bg(fill=note_fill)
                    ) as note_box:
                        for line in note_lines:
                            with HSplit().set_content_align('l').set_item_align('c').set_sep(10):
                                Spacer(8, 8).set_bg(RoundRectBg(fill=(255, 160, 190, 255), radius=4))
                                TextBox(line, TextStyle(font=DEFAULT_FONT, size=18, color=(95, 105, 130))).set_w(552)
                await build_character_mission_summary_panel(ctx, profile, overview)

            for section in overview['section_results']:
                section['theme_color'] = theme
                await build_character_mission_section(section)

    add_watermark(canvas)
    return await canvas.get_img()


# ======================= 处理逻辑 ======================= #

# 获取玩家挑战live信息，返回（rank, score, remain_jewel, remain_fragment）
def get_character_mission_master_overview_cache_path(region: str) -> str:
    return pjoin(SEKAI_DATA_DIR, "character_mission_master_overview", f"{region}.png")


def get_character_mission_master_overview_lock(region: str) -> asyncio.Lock:
    if region not in CHARACTER_MISSION_MASTER_OVERVIEW_LOCKS:
        CHARACTER_MISSION_MASTER_OVERVIEW_LOCKS[region] = asyncio.Lock()
    return CHARACTER_MISSION_MASTER_OVERVIEW_LOCKS[region]


async def get_character_mission_master_overview_data(ctx: SekaiHandlerContext) -> dict:
    grouped_rows = defaultdict(list)
    for row in await ctx.md.character_mission_v2_parameter_groups.get():
        grouped_rows[int(row['id'])].append(row)
    for rows in grouped_rows.values():
        rows.sort(key=lambda item: (int(item['requirement']), int(item['seq'])))

    sections = []
    total_stamp = 0
    total_milestones = 0
    task_count = 0
    for section in CHARACTER_MISSION_MASTER_SECTION_DEFS:
        section_tasks = []
        section_stamp = 0
        for title, groups in section['items']:
            task_count += 1
            cumulative = 0
            rows = []
            for group_id, marker in groups:
                assert_and_reply(group_id in grouped_rows, f"MasterData missing character mission parameter group {group_id}")
                group_cumulative = 0
                group_requirement_cumulative = 0
                for row in grouped_rows[group_id]:
                    exp = row.get('exp')
                    gain = int(exp or 0) if exp is not None else int(row.get('quantity', 0) or 0)
                    if int(row.get('exp', 0) or 0) == 0 and int(row.get('quantity', 0) or 0) > 0:
                        logger.warning(
                            f"character mission master overview: parameter group {group_id} requirement={row['requirement']} "
                            f"has exp=0 quantity={row['quantity']}, ignore quantity in stamp summary"
                        )
                    cumulative += gain
                    group_cumulative += gain
                    requirement = int(row['requirement'])
                    if marker:
                        group_requirement_cumulative += requirement
                        display_requirement = group_requirement_cumulative
                    else:
                        display_requirement = requirement
                    total_milestones += 1
                    rows.append({
                        'requirement': requirement,
                        'display_requirement': display_requirement,
                        'gain': gain,
                        'total': group_cumulative,
                        'marker': marker,
                    })
            section_stamp += cumulative
            section_tasks.append({
                'title': title,
                'rows': rows,
                'total_stamp': cumulative,
            })
        total_stamp += section_stamp
        sections.append({
            **section,
            'tasks': section_tasks,
            'total_stamp': section_stamp,
        })

    return {
        'sections': sections,
        'total_stamp': total_stamp,
        'task_count': task_count,
        'milestone_count': total_milestones,
    }


def get_character_mission_master_task_layout(task: dict) -> tuple[int, int]:
    max_requirement_len = max(
        [len(str(row.get('display_requirement', row['requirement']))) + (len(row['marker']) if row['marker'] else 0) for row in task['rows']],
        default=1,
    )
    if max_requirement_len <= 2:
        return 34, 20
    if max_requirement_len <= 3:
        return 40, 17
    if max_requirement_len <= 4:
        return 48, 14
    return 58, 11


def split_character_mission_master_rows(rows: list[dict], chunk_size: int) -> list[list[dict]]:
    chunks = []
    current = []
    ex_started = False
    for row in rows:
        if row['marker'] and current and not ex_started:
            chunks.append(current)
            current = []
            ex_started = True
        if len(current) >= chunk_size:
            chunks.append(current)
            current = []
        current.append(row)
        if row['marker']:
            ex_started = True
    if current:
        chunks.append(current)
    return chunks


async def build_character_mission_master_task_card(
    task: dict,
    color: Color,
    header_fill: Color,
) -> Frame:
    label_w = 30
    cell_w, chunk_size = get_character_mission_master_task_layout(task)
    chunk_rows_list = split_character_mission_master_rows(task['rows'], chunk_size)
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=(72, 86, 118))
    total_style = TextStyle(font=DEFAULT_BOLD_FONT, size=13, color=(*color[:3], 255))
    label_style = TextStyle(font=DEFAULT_BOLD_FONT, size=11, color=(80, 92, 122))
    cell_style = TextStyle(font=DEFAULT_BOLD_FONT, size=11, color=(95, 85, 96))
    card_fill = (255, 255, 255, 214)
    normal_cell_fill = soften_color(color, ratio=0.97, alpha=136)
    strong_cell_fill = soften_color(color, ratio=0.93, alpha=168)
    ex_cell_fill = soften_color(color, ratio=0.84, alpha=210)
    chunk_fill = soften_color(color, ratio=0.985, alpha=110)

    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(4).set_padding(8).set_bg(
        roundrect_bg(fill=card_fill)
    ) as ret:
        with HSplit().set_content_align('lt').set_item_align('c').set_sep(10):
            TextBox(task['title'], title_style)
            TextBox(f"总计 {task['total_stamp']} 章", total_style)

        for chunk_rows in chunk_rows_list:
            is_ex_chunk = bool(chunk_rows and chunk_rows[0]['marker'])
            top_label = "EX" if is_ex_chunk else "值"
            bottom_label = "章"
            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(3).set_padding((5, 4)).set_bg(
                roundrect_bg(fill=chunk_fill)
            ):
                with HSplit().set_content_align('lt').set_item_align('c').set_sep(4):
                    TextBox(top_label, label_style).set_w(label_w).set_content_align('c').set_padding((0, 2)).set_bg(
                        RoundRectBg(fill=header_fill, radius=8)
                    )
                    for row in chunk_rows:
                        requirement_text = str(row.get('display_requirement', row['requirement']))
                        cell_fill = ex_cell_fill if row['marker'] else normal_cell_fill
                        TextBox(requirement_text, cell_style).set_w(cell_w).set_content_align('c').set_padding((0, 2)).set_bg(
                            RoundRectBg(fill=cell_fill, radius=8)
                        )
                with HSplit().set_content_align('lt').set_item_align('c').set_sep(4):
                    TextBox(bottom_label, label_style).set_w(label_w).set_content_align('c').set_padding((0, 2)).set_bg(
                        RoundRectBg(fill=header_fill, radius=8)
                    )
                    for row in chunk_rows:
                        TextBox(str(row['total']), cell_style).set_w(cell_w).set_content_align('c').set_padding((0, 2)).set_bg(
                            RoundRectBg(fill=strong_cell_fill, radius=8)
                        )
    return ret


async def build_character_mission_master_section(section: dict) -> Frame:
    color = section['color']
    section_fill = soften_color(color, ratio=0.9, alpha=225)
    note_style = TextStyle(font=DEFAULT_FONT, size=17, color=(120, 120, 120))
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=WHITE)
    total_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(92, 96, 118))
    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_padding(18).set_bg(
        roundrect_bg(fill=section_fill)
    ) as ret:
        with HSplit().set_content_align('lt').set_item_align('c').set_sep(12):
            TextBox(section['title'], title_style).set_padding((18, 8)).set_bg(
                RoundRectBg(fill=color, radius=18)
            )
            TextBox(f"总计 {section['total_stamp']} 章", total_style)
        if section.get('note'):
            TextBox(section['note'], note_style)
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(10):
            for task in section['tasks']:
                await build_character_mission_master_task_card(task, color, section['header_fill'])
    return ret


async def compose_character_mission_master_overview_image(ctx: SekaiHandlerContext) -> Image.Image:
    overview = await get_character_mission_master_overview_data(ctx)
    card_fill = (255, 255, 255, 228)
    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=34, color=(70, 84, 116))
    subtitle_style = TextStyle(font=DEFAULT_FONT, size=18, color=(96, 104, 126))
    note_style = TextStyle(font=DEFAULT_FONT, size=17, color=(102, 110, 132))
    summary_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(84, 96, 126))

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_padding(20).set_bg(
                roundrect_bg(fill=card_fill)
            ):
                TextBox(f"{get_region_name(ctx.region)}印章任务总览", title_style)
                TextBox(
                    f"总印章: {overview['total_stamp']}",
                    summary_style,
                )
                TextBox(f"生成时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}", note_style)

            for section in overview['sections']:
                await build_character_mission_master_section(section)

    add_watermark(canvas)
    return await canvas.get_img()


async def render_character_mission_master_overview_cache(ctx: SekaiHandlerContext) -> str:
    cache_path = get_character_mission_master_overview_cache_path(ctx.region)
    async with get_character_mission_master_overview_lock(ctx.region):
        img = await compose_character_mission_master_overview_image(ctx)

        def _save():
            create_parent_folder(cache_path)
            img.save(cache_path)

        await run_in_pool(_save)
        logger.info(f"character mission master overview: rendered cache to {cache_path}")
    return cache_path


async def get_character_mission_master_overview_image_path(
    ctx: SekaiHandlerContext,
    refresh: bool = False,
) -> str:
    cache_path = get_character_mission_master_overview_cache_path(ctx.region)
    if refresh or not osp.exists(cache_path):
        return await render_character_mission_master_overview_cache(ctx)
    return cache_path


@RegionMasterDbManager.on_update()
async def refresh_character_mission_master_overview_on_masterdb_update(
    region: str, source: str,
    version: str, last_version: str,
    asset_version: str, last_asset_version: str,
):
    try:
        ctx = SekaiHandlerContext.from_region(region)
        await render_character_mission_master_overview_cache(ctx)
        logger.info(
            f"character mission master overview: auto refreshed for {region} after masterdb update "
            f"{last_version} -> {version} ({source})"
        )
    except Exception:
        logger.print_exc(f"character mission master overview: auto refresh failed for {region}")


async def get_user_challenge_live_info(ctx: SekaiHandlerContext, profile: dict) -> Dict[int, Tuple[int, int, int, int]]:
    # pjsk detail 也会复用这里，缺字段时按空列表降级，避免整图报错。
    challenge_info = {}
    challenge_results = profile.get('userChallengeLiveSoloResults', [])
    challenge_stages = profile.get('userChallengeLiveSoloStages', [])
    challenge_rewards = profile.get('userChallengeLiveSoloHighScoreRewards', [])
    for cid in range(1, 27):
        stages = find_by(challenge_stages, 'characterId', cid, mode='all')
        rank = max([stage['rank'] for stage in stages]) if stages else 0
        result = find_by(challenge_results, 'characterId', cid)
        score = result['highScore'] if result else 0
        remain_jewel, remain_fragment = 0, 0
        completed_reward_ids = [item['challengeLiveHighScoreRewardId'] for item in find_by(challenge_rewards, 'characterId', cid, mode='all')]
        for reward in await ctx.md.challenge_live_high_score_rewards.get():
            if reward['id'] in completed_reward_ids or reward['characterId'] != cid:
                continue
            res_box = await get_res_box_info(ctx, 'challenge_live_high_score', reward['resourceBoxId'])
            for res in res_box:
                if res['type'] == 'jewel':
                    remain_jewel += res['quantity']
                if res['type'] == 'material' and res['id'] == 15:
                    remain_fragment += res['quantity']
        challenge_info[cid] = (rank, score, remain_jewel, remain_fragment)
    return challenge_info

async def build_challenge_live_detail_section(ctx: SekaiHandlerContext, profile: dict):
    # 原挑战信息表格拆成 section，保留原命令绘图口径不变。
    challenge_info = await get_user_challenge_live_info(ctx, profile)

    header_h, row_h = 56, 48
    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(25, 25, 25, 255))
    text_style = TextStyle(font=DEFAULT_FONT, size=20, color=(50, 50, 50, 255))
    w1, w2, w3, w4, w5, w6 = 80, 80, 150, 300, 80, 80

    reward_items = await ctx.md.challenge_live_high_score_rewards.get()
    max_score = max([item['highScore'] for item in reward_items], default=1)

    with VSplit().set_content_align('c').set_item_align('c').set_sep(8).set_padding(16).set_bg(roundrect_bg()) as f:
        with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(header_h).set_padding(4).set_bg(roundrect_bg()):
            TextBox("角色", header_style).set_w(w1).set_content_align('c')
            TextBox("等级", header_style).set_w(w2).set_content_align('c')
            TextBox("分数", header_style).set_w(w3).set_content_align('c')
            TextBox(f"进度(上限{max_score//10000}w)", header_style).set_w(w4).set_content_align('c')
            with Frame().set_w(w5).set_content_align('c'):
                ImageBox(ctx.static_imgs.get("jewel.png"), size=(None, 40))
            with Frame().set_w(w6).set_content_align('c'):
                ImageBox(ctx.static_imgs.get("shard.png"), size=(None, 40))

        for cid in range(1, 27):
            bg_color = (255, 255, 255, 150) if cid % 2 == 0 else (255, 255, 255, 100)
            rank = str(challenge_info[cid][0]) if challenge_info[cid][0] else "-"
            score = str(challenge_info[cid][1]) if challenge_info[cid][1] else "-"
            jewel = str(challenge_info[cid][2])
            fragment = str(challenge_info[cid][3])
            with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(row_h).set_padding(4).set_bg(roundrect_bg(fill=bg_color)):
                with Frame().set_w(w1).set_content_align('c'):
                    ImageBox(get_chara_icon_by_chara_id(cid), size=(None, 40))
                TextBox(rank, text_style).set_w(w2).set_content_align('c')
                TextBox(score, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w3).set_content_align('c')

                with Frame().set_w(w4).set_content_align('lt'):
                    x = challenge_info[cid][1]
                    progress = max(min(x / max_score, 1), 0)
                    total_w, total_h, border = w4, 14, 2
                    progress_w = int((total_w - border * 2) * progress)
                    progress_h = total_h - border * 2
                    color = (255, 50, 50, 255)
                    if x > 250_0000:    color = (100, 255, 100, 255)
                    elif x > 200_0000:  color = (255, 255, 100, 255)
                    elif x > 150_0000:  color = (255, 200, 100, 255)
                    elif x > 100_0000:  color = (255, 150, 100, 255)
                    elif x > 50_0000:   color = (255, 100, 100, 255)
                    if progress > 0:
                        Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 255), radius=total_h//2))
                        Spacer(w=progress_w, h=progress_h).set_bg(RoundRectBg(fill=color, radius=(total_h-border)//2)).set_offset((border, border))

                        def draw_line(line_x: int):
                            p = line_x / max_score
                            if p <= 0 or p >= 1:
                                return
                            lx = int((total_w - border * 2) * p)
                            line_color = (100, 100, 100, 255) if line_x < x else (150, 150, 150, 255)
                            Spacer(w=1, h=total_h//2-1).set_bg(FillBg(line_color)).set_offset((border + lx - 1, total_h//2))
                        for line_x in range(0, max_score, 50_0000):
                            draw_line(line_x)
                    else:
                        Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 100), radius=total_h//2))

                TextBox(jewel, text_style).set_w(w5).set_content_align('c')
                TextBox(fragment, text_style).set_w(w6).set_content_align('c')
    return f

async def compose_challenge_live_detail_image(ctx: SekaiHandlerContext, qid: int) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx, qid, 
        filter=get_detailed_profile_card_filter('userChallengeLiveSoloResults','userChallengeLiveSoloStages','userChallengeLiveSoloHighScoreRewards'), 
        raise_exc=True)

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)
            await build_challenge_live_detail_section(ctx, profile)

    add_watermark(canvas)
    return await canvas.get_img()

# 获取玩家加成信息
async def get_user_power_bonus(ctx: SekaiHandlerContext, profile: dict) -> Dict[str, int]:
    # pjsk detail 复用加成计算时，缺失 suite 字段统一按 0 处理。
    # 获取区域道具
    area_items: List[dict] = []
    for user_area in profile.get('userAreas', []):
        for user_area_item in user_area.get('areaItems', []):
            item_id = user_area_item['areaItemId']
            lv = user_area_item['level']
            item = find_by(find_by(await ctx.md.area_item_levels.get(), 'areaItemId', item_id, mode='all'), 'level', lv)
            if item:
                area_items.append(item)

    # 角色加成 = 区域道具 + 角色等级 + 烤森家具
    chara_bonus = { i : {
        'area_item': 0,
        'rank': 0,
        'fixture': 0,
    } for i in range(1, 27) }
    for item in area_items:
        if item.get('targetGameCharacterId', "any") != "any":
            chara_bonus[item['targetGameCharacterId']]['area_item'] += item['power1BonusRate']
    for chara in profile.get('userCharacters', []):
        rank = find_by(await ctx.md.character_ranks.find_by('characterId', chara['characterId'], mode='all'), 'characterRank', chara['characterRank'])
        if rank:
            chara_bonus[chara['characterId']]['rank'] += rank['power1BonusRate']
    for fb in profile.get('userMysekaiFixtureGameCharacterPerformanceBonuses', []):
        chara_bonus[fb['gameCharacterId']]['fixture'] += fb['totalBonusRate'] * 0.1
    
    # 组合加成 = 区域道具 + 烤森门
    unit_bonus = { unit : {
        'area_item': 0,
        'gate': 0,
    } for unit in UNITS }
    for item in area_items:
        if item.get('targetUnit', "any") != "any":
            unit_bonus[item['targetUnit']]['area_item'] += item['power1BonusRate']
    max_bonus = 0
    for gate in profile.get('userMysekaiGates', []):
        gate_id = gate['mysekaiGateId']
        bonus = find_by(await ctx.md.mysekai_gate_levels.find_by('mysekaiGateId', gate_id, mode='all'), 'level', gate['mysekaiGateLevel'])
        if not bonus:
            continue
        unit_bonus[UNITS[gate_id - 1]]['gate'] += bonus['powerBonusRate']
        max_bonus = max(max_bonus, bonus['powerBonusRate'])
    unit_bonus[UNIT_VS]['gate'] += max_bonus

    # 属性加成 = 区域道具
    attr_bouns = { attr : {
        'area_item': 0,
    } for attr in CARD_ATTRS }
    for item in area_items:
        if item.get('targetCardAttr', "any") != "any":
            attr_bouns[item['targetCardAttr']]['area_item'] += item['power1BonusRate']

    for _, bonus in chara_bonus.items():
        bonus['total'] = sum(bonus.values())
    for _, bonus in unit_bonus.items():
        bonus['total'] = sum(bonus.values())
    for _, bonus in attr_bouns.items():
        bonus['total'] = sum(bonus.values())
    
    return {
        "chara": chara_bonus,
        "unit": unit_bonus,
        "attr": attr_bouns
    }

async def build_power_bonus_detail_section(ctx: SekaiHandlerContext, profile: dict):
    # 原加成信息表格拆成 section，供原命令和 pjsk detail 共用。
    bonus = await get_user_power_bonus(ctx, profile)
    chara_bonus = bonus['chara']
    unit_bonus = bonus['unit']
    attr_bonus = bonus['attr']

    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(25, 25, 25, 255))
    text_style = TextStyle(font=DEFAULT_FONT, size=16, color=(100, 100, 100, 255))

    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_item_bg(roundrect_bg()).set_bg(roundrect_bg()).set_padding(16) as f:
        cid_parts = [range(1, 5), range(5, 9), range(9, 13), range(13, 17), range(17, 21), range(21, 27)]
        for cids in cid_parts:
            with Grid(col_count=2).set_content_align('l').set_item_align('l').set_sep(20, 4).set_padding(16):
                for cid in cids:
                    with HSplit().set_content_align('l').set_item_align('l').set_sep(4):
                        ImageBox(get_chara_icon_by_chara_id(cid), size=(None, 40))
                        TextBox(f"{chara_bonus[cid]['total']:.1f}%", header_style).set_w(100).set_content_align('r').set_overflow('clip')
                        detail = f"区域道具{chara_bonus[cid]['area_item']:.1f}% + 角色等级{chara_bonus[cid]['rank']:.1f}% + 烤森玩偶{chara_bonus[cid]['fixture']:.1f}%"
                        TextBox(detail, text_style)

        with Grid(col_count=3).set_content_align('l').set_item_align('l').set_sep(20, 4).set_padding(16):
            for unit in UNITS:
                with HSplit().set_content_align('l').set_item_align('l').set_sep(4):
                    ImageBox(get_unit_icon(unit), size=(None, 40))
                    TextBox(f"{unit_bonus[unit]['total']:.1f}%", header_style).set_w(100).set_content_align('r').set_overflow('clip')
                    detail = f"区域道具{unit_bonus[unit]['area_item']:.1f}% + 烤森门{unit_bonus[unit]['gate']:.1f}%"
                    TextBox(detail, text_style)

        with Grid(col_count=5).set_content_align('l').set_item_align('l').set_sep(20, 4).set_padding(16):
            for attr in CARD_ATTRS:
                with HSplit().set_content_align('l').set_item_align('l').set_sep(4):
                    ImageBox(get_attr_icon(attr), size=(None, 40))
                    TextBox(f"{attr_bonus[attr]['total']:.1f}%", header_style).set_w(100).set_content_align('r').set_overflow('clip')
    return f

# 合成加成详情图片
async def compose_power_bonus_detail_image(ctx: SekaiHandlerContext, qid: int) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx, 
        qid, 
        filter=get_detailed_profile_card_filter('userAreas','userCharacters','userMysekaiFixtureGameCharacterPerformanceBonuses','userMysekaiGates'), 
        raise_exc=True)
    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)
            await build_power_bonus_detail_section(ctx, profile)

    add_watermark(canvas)
    return await canvas.get_img()

# 合成区域道具升级材料图片
async def compose_area_item_upgrade_materials_image(ctx: SekaiHandlerContext, qid: int, filter: AreaItemFilter) -> Image.Image:
    profile = None
    if qid:
        profile, pmsg = await get_detailed_profile(
            ctx, 
            qid, 
            filter=get_detailed_profile_card_filter('userMaterials','userGamedata','userAreas',),
            raise_exc=True, 
            ignore_hide=True)
        
    COIN_ID = -1
    user_materials: dict[int, int] = {}
    user_area_item_lvs: dict[int, int] = {}
    
    if profile:
        # 获取玩家材料（金币当作id=-1的材料）
        assert_and_reply('userMaterials' in profile, "你的Suite数据来源没有提供userMaterials数据（可能需要重传）")
        user_materials = {}
        user_materials[COIN_ID] = profile['userGamedata'].get('coin', 0)
        for item in profile.get('userMaterials', []):
            user_materials[item['materialId']] = item['quantity']
        # 获取玩家区域道具等级
        user_area_item_lvs = {}
        for area in profile.get('userAreas', []):
            for area_item in area.get('areaItems', []):
                user_area_item_lvs[area_item['areaItemId']] = area_item['level']

    # 筛选vs额外判断
    filter_piapro = False
    if filter.unit == 'piapro':
        filter.unit = None
        filter_piapro = True

    # 获取区域道具信息，同时筛选需要展示的区域道具id
    item_ids: set[int] = set()
    area_item_icons: dict[int, Image.Image] = {}
    area_item_target_icons: dict[int, Image.Image] = {}
    area_item_level_bonuses: dict[int, dict[int, float]] = {}
    area_item_max_levels: dict[int, int] = {}
    for item in await ctx.md.area_items.get():
        item_id, area_id, asset_name = item['id'], item['areaId'], item['assetbundleName']

        is_vs_item = False

        area_item_icons[item_id] = await ctx.rip.img(f"areaitem/{asset_name}/{asset_name}.png")
        for item_lv in await ctx.md.area_item_levels.find_by('areaItemId', item_id, mode='all'):
            area_item_level_bonuses.setdefault(item_id, {})[item_lv['level']] = item_lv['power1BonusRate']
            area_item_max_levels[item_id] = max(area_item_max_levels.get(item_id, 0), item_lv['level'])

            if item_id not in area_item_target_icons:
                if item_lv.get('targetUnit', 'any') != 'any':
                    area_item_target_icons[item_id] = get_unit_icon(item_lv['targetUnit'])
                    if item_lv['targetUnit'] == 'piapro':
                        if filter_piapro:
                            item_ids.add(item_id)
                        is_vs_item = True
                elif item_lv.get('targetGameCharacterId', 'any') != 'any':
                    area_item_target_icons[item_id] = get_chara_icon_by_chara_id(item_lv['targetGameCharacterId'])
                    if filter.cid and item_lv['targetGameCharacterId'] == filter.cid:
                        item_ids.add(item_id)
                    if item_lv['targetGameCharacterId'] in UNIT_CID_MAP['piapro']:
                        is_vs_item = True
                elif item_lv.get('targetCardAttr', 'any') != 'any':
                    area_item_target_icons[item_id] = get_attr_icon(item_lv['targetCardAttr'])
                    if filter.attr and item_lv['targetCardAttr'] == filter.attr:
                        item_ids.add(item_id)

        if filter.flower and area_id == FLOWER_AREA_ID:
            item_ids.add(item_id)
        if filter.tree and area_id == TREE_AREA_ID:
            item_ids.add(item_id)
        if filter.unit and area_id == UNIT_SEKAI_AREA_IDS[filter.unit] and not is_vs_item:
            item_ids.add(item_id)

    item_ids = sorted(item_ids)

    # 统计展示的最低等级
    user_area_item_lower_lv = None
    for item_id in item_ids:
        lv = user_area_item_lvs.get(item_id, 0)
        if user_area_item_lower_lv is None or lv < user_area_item_lower_lv:
            user_area_item_lower_lv = lv
    if user_area_item_lower_lv is None:
        user_area_item_lower_lv = 0

    # 获取区域道具等级对应的shopItem的resboxId ids[item_id][level] = resbox_id
    area_item_lv_shop_item_resbox_ids: dict[int, dict[int, int]] = {}
    for box_id, box in (await ctx.md.resource_boxes.get())['shop_item'].items():
        if details := box.get('details'):
            detail = details[0]
            res_type = detail.get('resourceType')
            res_id = detail.get('resourceId')
            res_lv = detail.get('resourceLevel')
            if res_type == 'area_item' and res_id in item_ids and res_lv <= area_item_max_levels.get(res_id, 0):
                area_item_lv_shop_item_resbox_ids.setdefault(res_id, {})[res_lv] = box_id
                
    # 获取区域道具升级材料列表 m[item_id][level][material_id] = quantity
    area_item_lv_materials: dict[int, dict[int, dict[int, int]]] = {}
    for item_id in item_ids:
        for lv, resbox_id in area_item_lv_shop_item_resbox_ids[item_id].items():
            for cost in (await ctx.md.shop_items.find_by_id(resbox_id)).get('costs', []):
                cost = cost['cost']
                res_id = cost['resourceId']
                if cost['resourceType'] == 'coin':
                    res_id = COIN_ID
                quantity = cost['quantity']
                area_item_lv_materials.setdefault(item_id, {}).setdefault(lv, {})[res_id] = quantity

    # 计算从玩家当前等级到目标等级所需材料（没有提供profile则从0累计）
    area_item_lv_sum_materials: dict[int, dict[int, dict[int, dict]]] = {}
    for item_id, lv_materials in area_item_lv_materials.items():
        user_lv = user_area_item_lvs.get(item_id, 0)
        sum_materials: dict[int, int] = {}
        # 枚举等级和材料
        for lv in range(user_lv + 1, area_item_max_levels[item_id] + 1):
            for mid, quantity in lv_materials[lv].items():
                sum_materials[mid] = sum_materials.get(mid, 0) + quantity
                area_item_lv_sum_materials.setdefault(item_id, {}).setdefault(lv, {})[mid] = sum_materials[mid]

    def get_quant_text(q: int) -> str:
        if q >= 10000000:
            return f"{q//10000000}kw"
        elif q >= 10000:
            x, y = q//10000, (q%10000)//1000
            if x < 10 and y > 0:
                return f"{x}w{y}"
            return f"{x}w"
        elif q >= 1000:
            x, y = q//1000, (q%1000)//100
            if x < 10 and y > 0:
                return f"{x}k{y}"
            return f"{x}k"
        else:
            return str(q)
    
    # 绘图
    gray_color, red_color, green_color = (50, 50, 50), (200, 0, 0), (0, 200, 0)
    ok_color = green_color if profile else gray_color
    no_color = red_color if profile else gray_color
    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            if profile:
                await get_detailed_profile_card(ctx, profile, pmsg)

            with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16).set_bg(roundrect_bg()).set_padding(8):
                for item_id, lv_materials in area_item_lv_materials.items():
                    lv_sum_materials = area_item_lv_sum_materials.get(item_id, {})
                    current_lv = user_area_item_lvs.get(item_id, 0)
                    # 每个道具的列
                    with VSplit().set_content_align('l').set_item_align('l').set_sep(8).set_item_bg(roundrect_bg()).set_padding(8):
                        # 列头
                        with HSplit().set_content_align('c').set_item_align('c').set_omit_parent_bg(True):
                            ImageBox(area_item_target_icons.get(item_id, UNKNOWN_IMG), size=(None, 64))
                            ImageBox(area_item_icons.get(item_id, UNKNOWN_IMG), size=(128, 64), image_size_mode='fit') \
                                .set_content_align('c')
                            if current_lv:
                                TextBox(f"Lv.{current_lv}", TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=gray_color))

                        lv_can_upgrade = True
                        for lv in range(user_area_item_lower_lv + 1, area_item_max_levels[item_id] + 1):
                            # 统计道具是否足够
                            if lv > current_lv:
                                material_is_enough: dict[int, bool] = {}
                                for mid, quantity in lv_sum_materials[lv].items():
                                    material_is_enough[mid] = user_materials.get(mid, 0) >= quantity
                                lv_can_upgrade = lv_can_upgrade and all(material_is_enough.values())

                            # 列项
                            with HSplit().set_content_align('l').set_item_align('l').set_sep(8).set_padding(8):
                                bonus_text = f"+{area_item_level_bonuses[item_id][lv]:.1f}%"
                                with VSplit().set_content_align('c').set_item_align('c').set_sep(4):
                                    color = ok_color if lv_can_upgrade else no_color
                                    if lv <= current_lv:
                                        color = gray_color
                                    TextBox(f"{lv}", TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=color))
                                    TextBox(bonus_text, TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=gray_color)).set_w(64)

                                if lv <= current_lv:
                                    with VSplit().set_content_align('c').set_item_align('c').set_sep(4):
                                        Spacer(w=64, h=64)
                                        TextBox(" ", TextStyle(font=DEFAULT_BOLD_FONT, size=15, color=gray_color))
                                else:
                                    for mid, quantity in lv_materials[lv].items():
                                        with VSplit().set_content_align('c').set_item_align('c').set_sep(4):
                                            material_icon = await get_res_icon(ctx, 'coin' if mid == COIN_ID else 'material', mid)
                                            quantity_text = get_quant_text(quantity)
                                            have_text = get_quant_text(user_materials.get(mid, 0))
                                            sum_text = get_quant_text(lv_sum_materials[lv][mid])
                                            with Frame():
                                                sz = 64
                                                ImageBox(material_icon, size=(sz, sz))
                                                TextBox(f"x{quantity_text}", TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=(50, 50, 50))) \
                                                    .set_offset((sz, sz)).set_offset_anchor('rb')
                                            color = ok_color if material_is_enough.get(mid) else no_color
                                            text = f"{have_text}/{sum_text}" if profile else f"{sum_text}"
                                            TextBox(text, TextStyle(font=DEFAULT_BOLD_FONT, size=15, color=color))

    add_watermark(canvas)

    # 缓存full查询
    cache_key = None
    if profile is None:
        cache_key = f"{ctx.region}_area_item_{filter.unit}_{filter.cid}_{filter.attr}_{filter.flower}_{filter.tree}"
    return await canvas.get_img(scale=0.75, cache_key=cache_key)

# 合成羁绊等级图片
async def compose_bonds_image(ctx: SekaiHandlerContext, qid: int, cid: int | None) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx, 
        qid, 
        filter=get_detailed_profile_card_filter('userBonds', 'userCharacters'),
        raise_exc=True)
    
    user_bonds = profile.get('userBonds')
    assert_and_reply(user_bonds, "你的Suite数据来源没有提供userBonds数据")

    def extract_cid_from_bgid(bgid: int) -> tuple[int, int]:
        return bgid // 100 % 100, bgid % 100

    # 收集羁绊等级需要的经验信息
    bond_level_total_exps: dict[int, int] = {}
    max_level = 0
    for item in await ctx.md.levels.find_by('levelType', 'bonds', mode='all'):
        lv, exp = item['level'], item['totalExp']
        bond_level_total_exps[lv] = exp
        max_level = max(max_level, lv)
    
    # 收集所有的羁绊角色
    bonds: dict[tuple[int, int], dict] = {}
    for item in await ctx.md.bonds.get():
        c1, c2 = extract_cid_from_bgid(item['groupId'])
        bonds[(c1, c2)] = {}
    
    # 收集用户的羁绊等级
    for item in user_bonds:
        c1, c2 = extract_cid_from_bgid(item['bondsGroupId'])
        bonds[(c1, c2)] = item

    # 收集用户的角色等级
    character_ranks = {}
    for item in profile.get('userCharacters', []):
        character_ranks[item['characterId']] = item['characterRank']

    if cid is not None:
        # 只保留与cid相关的羁绊，并且调整cid在前
        for c1, c2 in list(bonds.keys()):
            if c1 != cid and c2 != cid:
                bonds.pop((c1, c2))
            elif c1 != cid:
                bonds[(cid, c1)] = bonds.pop((c1, c2))
        bond_keys = [(cid, i) for i in range(1, 27) if i != cid]
    else:
        # 保留羁绊等级前topk的角色，并调整角色等级高的在前
        TOPK = 25
        bond_keys = sorted(bonds.keys(), key=lambda k: bonds[k]['rank'] if bonds[k] else 0, reverse=True)[:TOPK]
        bond_keys = [(x, y) if character_ranks.get(x, 0) >= character_ranks.get(y, 0) else (y, x) for x, y in bond_keys]
        bonds = { k: bonds.get(k, bonds.get((k[1], k[0]), 0)) for k in bond_keys }
        
    header_h, row_h = 56, 48
    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(25, 25, 25, 255))
    text_style = TextStyle(font=DEFAULT_FONT, size=20, color=(50, 50, 50, 255))
    w1, w2, w3, w4, w5 = 100, 120, 100, 350, 150

    # 绘图
    async def get_chara_color(c: int):
        return color_code_to_rgb((await ctx.md.game_character_units.find_by_id(c))['colorCode'])

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)
            with VSplit().set_content_align('l').set_item_align('l').set_sep(8).set_padding(16).set_bg(roundrect_bg()):
                # 标题
                with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(header_h).set_padding(4).set_bg(roundrect_bg()):
                    TextBox("角色", header_style).set_w(w1).set_content_align('c')
                    TextBox("角色等级", header_style).set_w(w2).set_content_align('c')
                    TextBox("羁绊等级", header_style).set_w(w3).set_content_align('c')
                    TextBox(f"进度(上限{max_level}级)", header_style).set_w(w4).set_content_align('c')
                    TextBox("升级经验", header_style).set_w(w5).set_content_align('c')

                # 项目
                index = 0
                for key in bond_keys:
                    c1, c2 = key
                    bg_color = (255, 255, 255, 150) if index % 2 == 0 else (255, 255, 255, 100)
                    index += 1

                    has_bond = key in bonds
                    
                    level = 0
                    if has_bond and bonds[key]:
                        level = bonds[key]['rank']

                    level_text, need_exp_text = "-", "-"
                    if has_bond:
                        if level:
                            level_text = str(level)
                        if level == max_level:
                            need_exp_text = "MAX"
                        elif level > 0:
                            exp = bonds[key]['exp'] if bonds[key] else 0
                            level_exp = bond_level_total_exps[level + 1] - bond_level_total_exps[level]
                            need_exp_text = str(level_exp - exp)

                    color1 = await get_chara_color(c1)
                    color2 = await get_chara_color(c2)

                    crank1 = character_ranks.get(c1, 0)
                    crank2 = character_ranks.get(c2, 0)
                    chara_rank_text = f"{crank1} & {crank2}"

                    level_color = (50, 50, 50, 255)
                    if min(crank1, crank2) <= level and level < max_level:
                        level_color = (150, 0, 0, 255)

                    with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(row_h).set_padding(4).set_bg(roundrect_bg(fill=bg_color)):
                        with Frame().set_w(w1).set_content_align('c'):
                            ImageBox(get_chara_icon_by_chara_id(c1),   size=(None, 40)).set_offset((-13, 0))
                            ImageBox(get_chara_icon_by_chara_id(c2),    size=(None, 40)).set_offset((13, 0))

                        TextBox(chara_rank_text, text_style.replace(font=DEFAULT_BOLD_FONT, color=level_color)).set_w(w2).set_content_align('c')
                        TextBox(level_text, text_style.replace(font=DEFAULT_BOLD_FONT, color=level_color)).set_w(w3).set_content_align('c')

                        with Frame().set_w(w4).set_content_align('lt'):
                            x = level
                            progress = max(min(x / max_level, 1), 0)
                            total_w, total_h, border = w4, 14, 2
                            progress_w = int((total_w - border * 2) * progress)
                            progress_h = total_h - border * 2
                            color = LinearGradient(c1=color1, c2=color2, p1=(0, 0.5), p2=(1, 0.5))
                            if has_bond and progress > 0:
                                Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 255), radius=total_h//2))
                                Spacer(w=progress_w, h=progress_h).set_bg(RoundRectBg(fill=color, radius=(total_h-border)//2)).set_offset((border, border))

                                def draw_line(line_x: int):
                                    p = line_x / max_level
                                    if p <= 0 or p >= 1: return
                                    lx = int((total_w - border * 2) * p)
                                    color = (100, 100, 100, 255) if line_x < x else (150, 150, 150, 255)
                                    Spacer(w=1, h=total_h//2-1).set_bg(FillBg(color)).set_offset((border + lx - 1, total_h//2))
                                for line_x in range(0, max_level, 10):
                                    draw_line(line_x)
                            else:
                                Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 100), radius=total_h//2))

                        TextBox(need_exp_text, text_style).set_w(w5).set_content_align('c')

    add_watermark(canvas)
    return await canvas.get_img()

# 合成队长次数图片
async def compose_leader_count_image(ctx: SekaiHandlerContext, qid: int) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx, 
        qid, 
        filter=get_detailed_profile_card_filter('userCharacterMissionV2s', 'userCharacterMissionV2Statuses'),
        raise_exc=True)

    ucms = profile.get('userCharacterMissionV2s')
    ucm_ss = profile.get('userCharacterMissionV2Statuses')
    assert_and_reply(ucms, "你的Suite数据来源没有提供userCharacterMissionV2s数据")
    assert_and_reply(ucm_ss, "你的Suite数据来源没有提供userCharacterMissionV2Statuses数据")

    # 获取游玩次数上限和ex每次次数
    max_playcount = 0
    ex_seq_pc_list: list[tuple[int, int]] = []
    for item in await ctx.md.character_mission_v2_parameter_groups.find_by('id', 1, mode='all'):
        max_playcount = max(max_playcount, item['requirement'])
    for item in await ctx.md.character_mission_v2_parameter_groups.find_by('id', 101, mode='all'):
        ex_seq_pc_list.append((item['seq'], item['requirement']))
    ex_seq_pc_list.append((100000, 0))
    ex_seq_pc_list.sort()
    
    # 收集用户游玩次数
    playcounts: dict[int, int] = {}
    playcounts_ex: dict[int, int] = {}
    ex_level: dict[int, int] = {}
    for item in find_by(ucms, 'characterMissionType', 'play_live', mode='all'):
        playcounts[item['characterId']] = item['progress']
    for item in find_by(ucms, 'characterMissionType', 'play_live_ex', mode='all'):
        playcounts_ex[item['characterId']] = item['progress']
        ex_level[item['characterId']] = 0
    for item in find_by(ucm_ss, 'parameterGroupId', 101, mode='all'):
        cid, seq = item['characterId'], item['seq']
        ex_level[cid] = max(ex_level.get(cid, 0), seq)
        for i in range(len(ex_seq_pc_list)):
            if ex_seq_pc_list[i+1][0] > seq:
                playcounts_ex[cid] += ex_seq_pc_list[i][1]
                break
        
    header_h, row_h = 56, 48
    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(25, 25, 25, 255))
    text_style = TextStyle(font=DEFAULT_FONT, size=20, color=(50, 50, 50, 255))
    w1, w2, w3, w4, w5 = 80, 100, 100, 100, 350

    # 绘图
    async def get_chara_color(c: int):
        return color_code_to_rgb((await ctx.md.game_character_units.find_by_id(c))['colorCode'])

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)
            with VSplit().set_content_align('l').set_item_align('l').set_sep(8).set_padding(16).set_bg(roundrect_bg()):

                # 标题
                with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(header_h).set_padding(4).set_bg(roundrect_bg()):
                    TextBox("角色", header_style).set_w(w1).set_content_align('c')
                    TextBox("队长次数", header_style).set_w(w2).set_content_align('c')
                    TextBox("EX等级", header_style).set_w(w3).set_content_align('c')
                    TextBox("EX次数", header_style).set_w(w4).set_content_align('c')
                    TextBox(f"进度(上限{max_playcount})", header_style).set_w(w5).set_content_align('c')

                # 项目
                index = 0
                for cid in range(1, 27):
                    bg_color = (255, 255, 255, 150) if index % 2 == 0 else (255, 255, 255, 100)
                    index += 1

                    pc = 0 if cid not in playcounts else playcounts[cid]
                    pc_text = "-" if cid not in playcounts else str(playcounts[cid])
                    pc_ex_text = "-" if cid not in playcounts_ex else str(playcounts_ex[cid])
                    ex_level_text = "-" if cid not in ex_level else f"x{ex_level[cid]}"

                    with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(row_h).set_padding(4).set_bg(roundrect_bg(fill=bg_color)):
                        with Frame().set_w(w1).set_content_align('c'):
                            ImageBox(get_chara_icon_by_chara_id(cid), size=(None, 40))

                        TextBox(pc_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w2).set_content_align('c')
                        TextBox(ex_level_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w3).set_content_align('c')
                        TextBox(pc_ex_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w4).set_content_align('c')

                        with Frame().set_w(w5).set_content_align('lt'):
                            x = pc
                            progress = max(min(x / max_playcount, 1), 0)
                            total_w, total_h, border = w5, 14, 2
                            progress_w = int((total_w - border * 2) * progress)
                            progress_h = total_h - border * 2
                            color = (255, 50, 50, 255)
                            if x > 50000:    color = (100, 255, 100, 255)
                            elif x > 40000:  color = (255, 255, 100, 255)
                            elif x > 30000:  color = (255, 200, 100, 255)
                            elif x > 20000:  color = (255, 150, 100, 255)
                            elif x > 10000:  color = (255, 100, 100, 255)
                            if progress > 0:
                                Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 255), radius=total_h//2))
                                Spacer(w=progress_w, h=progress_h).set_bg(RoundRectBg(fill=color, radius=(total_h-border)//2)).set_offset((border, border))

                                def draw_line(line_x: int):
                                    p = line_x / max_playcount
                                    if p <= 0 or p >= 1: return
                                    lx = int((total_w - border * 2) * p)
                                    color = (100, 100, 100, 255) if line_x < x else (150, 150, 150, 255)
                                    Spacer(w=1, h=total_h//2-1).set_bg(FillBg(color)).set_offset((border + lx - 1, total_h//2))
                                for line_x in range(0, max_playcount, 10000):
                                    draw_line(line_x)
                            else:
                                Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 100), radius=total_h//2))

    add_watermark(canvas)
    return await canvas.get_img()

# 合成材料信息图片
async def compose_material_info_image(ctx: SekaiHandlerContext, qid: int, show_all: bool = False) -> Image.Image:
    profile, pmsg = await get_detailed_profile(
        ctx,
        qid,
        filter=get_detailed_profile_card_filter('userMaterials', 'userGamedata'),
        raise_exc=True,
        ignore_hide=True,
    )

    assert_and_reply('userMaterials' in profile, "你的Suite数据来源没有提供userMaterials数据。")
    assert_and_reply('userGamedata' in profile, "你的Suite数据来源没有提供userGamedata数据。")

    user_materials = profile['userMaterials']
    if not user_materials:
        raise ReplyException("你的账号中没有任何材料记录，或Suite数据来源未提供userMaterials数据。")

    materials_dict = {
        item['id']: item
        for item in await RegionMasterDataWrapper(ctx.region, "materials").get()
    }

    display_items = []

    coin_quantity = profile['userGamedata'].get('coin', 0)
    if coin_quantity > 0 or show_all:
        display_items.append({
            'id': -1,
            'type': 'coin',
            'name': '金币',
            'quantity': coin_quantity,
            'seq': -1,
        })

    for um in user_materials:
        quantity = um['quantity']
        if quantity == 0 and not show_all:
            continue

        mat_id = um['materialId']
        material = materials_dict.get(mat_id)
        display_items.append({
            'id': mat_id,
            'type': 'material',
            'name': material['name'] if material and material.get('name') else f"未知材料({mat_id})",
            'quantity': quantity,
            'seq': material.get('seq', 999999) if material else 999999,
        })

    if not display_items:
        raise ReplyException("你当前没有任何持有数量大于0的材料。\n(可使用\"/材料信息 all\"查看所有历史获取记录)")

    display_items.sort(key=lambda x: (x['seq'], x['id']))

    icons = await batch_gather(*[get_res_icon(ctx, item['type'], item['id']) for item in display_items])
    for item, icon in zip(display_items, icons):
        item['icon'] = icon

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, pmsg)

            tip_text = "已隐藏数量为0的材料 可添加 all 参数查看所有记录" if not show_all else "当前已显示所有历史获取过的材料（包含数量为0）"
            TextBox(tip_text, TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(50, 50, 50))).set_bg(roundrect_bg()).set_padding(12)

            with Grid(col_count=4).set_sep(12, 12).set_item_align('lt').set_content_align('lt'):
                for item in display_items:
                    with HSplit().set_content_align('l').set_item_align('c').set_sep(10).set_padding(10).set_bg(roundrect_bg()).set_size((280, 80)):
                        ImageBox(item['icon'], size=(56, 56), use_alphablend=True)
                        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(6):
                            name_len = get_str_display_length(item['name'])
                            name_font_size = 18 if name_len <= 16 else 14
                            TextBox(item['name'], TextStyle(font=DEFAULT_BOLD_FONT, size=name_font_size, color=(70, 60, 80)), overflow='clip').set_w(190)
                            TextBox(f"x{item['quantity']}", TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=(50, 40, 60)))

    add_watermark(canvas)
    return await canvas.get_img()



# ======================= 指令处理 ======================= #

# 挑战信息
pjsk_challenge_info = SekaiCmdHandler([
    "/pjsk challenge info", "/pjsk_challenge_info",
    "/挑战信息", "/挑战详情", "/挑战进度", "/挑战一览", "/每日挑战", 
])
pjsk_challenge_info.check_cdrate(cd).check_wblist(gbl)
@pjsk_challenge_info.handle()
async def _(ctx: SekaiHandlerContext):
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_challenge_live_detail_image(ctx, ctx.user_id),
        low_quality=True,
    ))


# 加成信息
pjsk_power_bonus_info = SekaiCmdHandler([
    "/pjsk power bonus info", "/pjsk_power_bonus_info",
    "/加成信息", "/加成详情", "/加成进度", "/加成一览", "/角色加成",
])
pjsk_power_bonus_info.check_cdrate(cd).check_wblist(gbl)
@pjsk_power_bonus_info.handle()
async def _(ctx: SekaiHandlerContext):
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_power_bonus_detail_image(ctx, ctx.user_id),
        low_quality=True,
    ))


# 查询区域道具升级材料
pjsk_area_item = SekaiCmdHandler([
    "/pjsk area item", "/area item",
    "/区域道具", "/区域道具升级", "/区域道具升级材料",
])
pjsk_area_item.check_cdrate(cd).check_wblist(gbl)
@pjsk_area_item.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()

    HELP_TEXT = f"""
可用参数: 团名/角色名/属性/树/花
加上"all"可以查询所有级别材料，不加则查询你的账号的升级情况，示例：
"{ctx.original_trigger_cmd} 树" 所有树
"{ctx.original_trigger_cmd} miku" miku的道具
"{ctx.original_trigger_cmd} 25h" 25的SEKAI里的所有区域道具
"{ctx.original_trigger_cmd} miku all" miku的道具所有等级
""".strip()

    qid = ctx.user_id
    for keyword in ('all', 'full'):
        if keyword in args:
            qid = None
            args = args.replace(keyword, '').strip()
            break

    tree = False
    for keyword in ('树',):
        if keyword in args:
            tree = True
            args = args.replace(keyword, '').strip()
            break
    flower = False
    for keyword in ('花',):
        if keyword in args:
            flower = True
            args = args.replace(keyword, '').strip()
            break
    unit, args = extract_unit(args)
    attr, args = extract_card_attr(args)
    cid = get_cid_by_nickname(args)

    assert_and_reply(unit or attr or cid or tree or flower, HELP_TEXT)

    filter = AreaItemFilter(
        unit=unit,
        attr=attr,
        cid=cid,
        tree=tree,
        flower=flower,
    )
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_area_item_upgrade_materials_image(ctx, qid, filter),
        low_quality=True,
    ))


# 查询羁绊等级
pjsk_bonds = SekaiCmdHandler([
    "/pjsk bonds", "/pjsk bond",
    "/羁绊", "/羁绊等级", "/角色羁绊", "/羁绊信息", 
    "/牵绊等级", "/牵绊", "/角色牵绊", "/牵绊信息",
])
pjsk_bonds.check_cdrate(cd).check_wblist(gbl)
@pjsk_bonds.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()

    if args:
        cid = get_cid_by_nickname(args)
        assert_and_reply(cid is not None, f"请指定其中一个角色名称")
    else:
        cid = None

    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_bonds_image(ctx, ctx.user_id, cid),
        low_quality=True,
    ))

# 查询角色养成
pjsk_character_mission = SekaiCmdHandler([
    "/pjskcr", "/pjsk cr", "/pjsk character mission",
    "/角色等级", "/印章任务", "/角色养成",
])
pjsk_character_mission.check_cdrate(cd).check_wblist(gbl)
@pjsk_character_mission.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    cid = None
    if args:
        cid = get_cid_by_nickname(args)
        assert_and_reply(cid is not None, f"无法识别角色名: {args}")
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_character_mission_image(ctx, ctx.user_id, cid),
        low_quality=True,
    ))

# 查询印章任务总览
pjsk_character_mission_master_overview = SekaiCmdHandler([
    "/pjsk character mission master", "/pjsk character mission overview",
    "/pjsk任务总览", "/角色等级任务", "/等级任务总览",
], parse_uid_arg=False)
pjsk_character_mission_master_overview.check_cdrate(cd).check_wblist(gbl)
@pjsk_character_mission_master_overview.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    assert_and_reply(not args, f"无法解析的参数 \"{args}\"")
    image_path = await get_character_mission_master_overview_image_path(ctx, refresh=False)
    return await ctx.asend_reply_msg(await get_image_cq(open_image(image_path), low_quality=True))

#强制刷新印章任务总览
pjsk_character_mission_master_overview_refresh = SekaiCmdHandler([
    "/pjsk mission refresh", "/pjsk character mission overview refresh",
    "/pjsk任务刷新",
], parse_uid_arg=False)
pjsk_character_mission_master_overview_refresh.check_cdrate(cd).check_wblist(gbl)
@pjsk_character_mission_master_overview_refresh.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip()
    assert_and_reply(not args, f"无法解析的参数 \"{args}\"")
    image_path = await get_character_mission_master_overview_image_path(ctx, refresh=True)
    msg = await get_image_cq(open_image(image_path), low_quality=True)
    return await ctx.asend_reply_msg(f"已按当前 MasterData 强制重画印章任务总览\n{msg}")

# 查询队长次数
pjsk_leader_count = SekaiCmdHandler([
    "/pjsk leader count",
    "/队长次数", "/角色次数", "/队长游玩次数", "/角色游玩次数",
])
pjsk_leader_count.check_cdrate(cd).check_wblist(gbl)
@pjsk_leader_count.handle()
async def _(ctx: SekaiHandlerContext):
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_leader_count_image(ctx, ctx.user_id),
        low_quality=True,
    ))


# 查询材料信息
pjsk_material_info = SekaiCmdHandler([
    "/pjsk material", "/材料信息", "/pjsk材料",
])
pjsk_material_info.check_cdrate(cd).check_wblist(gbl)
@pjsk_material_info.handle()
async def _(ctx: SekaiHandlerContext):
    args = ctx.get_args().strip().lower()
    show_all = False
    for keyword in ('all', 'full'):
        if keyword in args:
            show_all = True
            args = args.replace(keyword, '', 1).strip()
            break
    assert_and_reply(not args, f"无法解析的参数 \"{args}\"")
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_material_info_image(ctx, ctx.user_id, show_all),
        low_quality=True,
    ))


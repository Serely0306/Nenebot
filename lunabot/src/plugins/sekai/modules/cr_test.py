from __future__ import annotations

import math

from ...utils import *
from ..common import *
from ..handler import *
from ..asset import *
from ..draw import *
from .profile import (
    get_detailed_profile,
    get_detailed_profile_card,
    get_detailed_profile_card_filter,
)


CHAR_MISSION_SHORT_NAMES = {
    "play_live": "队长次数",
    "play_live_ex": "队长次数(EX)",
    "waiting_room": "休息室次数",
    "waiting_room_ex": "休息室次数(EX)",
    "collect_costume_3d": "服装",
    "collect_stamp": "表情",
    "read_area_talk": "区域对话",
    "read_card_episode_first": "卡面剧情前篇",
    "read_card_episode_second": "卡面剧情后篇",
    "collect_another_vocal": "Another Vocal",
    "area_item_level_up_character": "单人家具升级次数",
    "area_item_level_up_unit": "团家具升级次数",
    "area_item_level_up_reality_world": "属性道具（树&花）升级次数",
    "collect_member": "卡面",
    "skill_level_up_rare": "技能等级升级次数（★4&生日卡）",
    "skill_level_up_standard": "技能等级升级次数（★1~★3）",
    "master_rank_up_rare": "专精等级升级次数（★4&生日卡）",
    "master_rank_up_standard": "专精等级升级次数（★1~★3）",
    "collect_character_archive_voice": "台词",
    "collect_mysekai_fixture": "MySekai家具数量",
    "collect_mysekai_canvas": "MySekai画布数量",
    "read_mysekai_fixture_unique_character_talk": "MySekai对话",
}

CHAR_MISSION_EX_TYPES = {"play_live_ex", "waiting_room_ex"}
CHAR_MISSION_EX_BASE_TYPES = {"play_live", "waiting_room"}

CHARACTER_RANK_ALL_KEYWORDS = ("all", "全部", "全量", "总表", "表格")
CHARACTER_RANK_MISSION_TYPE_ALIASES = {
    "play_live": ["队长次数", "角色次数", "队长游玩次数", "角色游玩次数", "队长", "队长次数ex", "队长次数(ex)", "队长次数（ex）", "角色次数ex"],
    "waiting_room": ["休息室次数", "休息室", "控制室", "休息室次数ex", "休息室次数(ex)", "休息室次数（ex）"],
    "collect_costume_3d": ["服装", "衣装", "服装数量", "衣装数量"],
    "collect_stamp": ["表情", "贴纸", "贴纸数量", "表情数量"],
    "read_area_talk": ["区域对话"],
    "read_card_episode_first": ["卡面剧情前篇", "前篇", "前编"],
    "read_card_episode_second": ["卡面剧情后篇", "后篇", "后编"],
    "collect_another_vocal": ["another vocal", "anvo"],
    "area_item_level_up_character": ["单人家具升级次数", "单人家具", "单人道具"],
    "area_item_level_up_unit": ["团家具升级次数", "团家具"],
    "area_item_level_up_reality_world": ["属性道具（树&花）升级次数", "树花", "属性家具", "属性道具", "植物"],
    "collect_member": ["卡面", "图鉴", "成员"],
    "skill_level_up_rare": ["技能等级升级次数（★4&生日卡）", "4星技能", "四星技能", "四星slv", "4星slv"],
    "skill_level_up_standard": ["技能等级升级次数（★1~★3）", "低星技能", "低星slv"],
    "master_rank_up_rare": ["专精等级升级次数（★4&生日卡）", "4星专精", "四星专精", "四星突破", "4星突破", "4星mr", "四星mr"],
    "master_rank_up_standard": ["专精等级升级次数（★1~★3）", "低星专精", "低星突破", "低星mr"],
    "collect_character_archive_voice": ["台词", "语音"],
    "collect_mysekai_fixture": ["mysekai家具数量", "ms家具", "烤森家具"],
    "collect_mysekai_canvas": ["mysekai画布数量", "ms画布", "烤森画布"],
    "read_mysekai_fixture_unique_character_talk": ["mysekai对话", "ms对话", "烤森对话"],
}
for mission_type, short_name in CHAR_MISSION_SHORT_NAMES.items():
    normalized = short_name.lower().replace(" ", "").replace("（", "(").replace("）", ")")
    CHARACTER_RANK_MISSION_TYPE_ALIASES.setdefault(mission_type, []).append(normalized)
    CHARACTER_RANK_MISSION_TYPE_ALIASES[mission_type].append(mission_type.lower())


def _normalize_cr_mission_query(text: str) -> str:
    return text.strip().lower().replace(" ", "").replace("（", "(").replace("）", ")")


def extract_cr_test_all_flag(args: str) -> tuple[bool, str]:
    normalized = _normalize_cr_mission_query(args)
    for keyword in CHARACTER_RANK_ALL_KEYWORDS:
        if keyword in normalized:
            return True, args.replace(keyword, "", 1).strip()
    return False, args.strip()


def extract_cr_test_mission_type(args: str) -> tuple[str | None, str]:
    normalized = _normalize_cr_mission_query(args)
    pairs: list[tuple[str, str]] = []
    for mission_type, aliases in CHARACTER_RANK_MISSION_TYPE_ALIASES.items():
        for alias in aliases:
            pairs.append((mission_type, alias))
    pairs.sort(key=lambda x: len(x[1]), reverse=True)
    for mission_type, alias in pairs:
        if alias in normalized:
            return mission_type, ""
    return None, args.strip()


def _char_mission_short_name(mission_type: str) -> str:
    return CHAR_MISSION_SHORT_NAMES.get(mission_type, mission_type)


def _get_pg_requirement_by_seq(
    pg_seq_requirements: dict[int, list[tuple[int, int]]],
    parameter_group_id: int,
    seq: int,
) -> int:
    if seq <= 0:
        return 0
    req = 0
    for item_seq, item_req in pg_seq_requirements.get(parameter_group_id, []):
        if item_seq > seq:
            break
        req = item_req
    return req


def _calc_mission_percent(current: int, upper: int | None) -> str:
    if upper is None or upper <= 0:
        return "-"
    return f"{min(current / upper * 100, 100.0):.1f}%"


def _draw_single_progress(
    line_title: str,
    current: int,
    upper: int | None,
    ratio: float,
    bar_width: int,
    bar_color: Color,
    title_size: int = 16,
    title_align: str = "l",
    title_badge: str | None = None,
    next_need: int | None = None,
    next_exp: int | None = None,
):
    style_title = TextStyle(font=DEFAULT_BOLD_FONT, size=title_size, color=(35, 35, 35, 255))
    style_text = TextStyle(font=DEFAULT_FONT, size=15, color=(55, 55, 55, 255))

    if line_title:
        if title_badge:
            with Frame().set_w(bar_width).set_content_align(title_align):
                with HSplit().set_content_align(title_align).set_item_align('c').set_sep(8):
                    TextBox(line_title, style_title)
                    TextBox(title_badge, TextStyle(font=DEFAULT_BOLD_FONT, size=16, color=(55, 55, 55, 255))) \
                        .set_bg(roundrect_bg(fill=(255, 255, 255, 180), radius=8)).set_padding((8, 2))
        else:
            TextBox(line_title, style_title).set_w(bar_width).set_content_align(title_align)

        Spacer(w=bar_width, h=4)

    raw_ratio = ratio
    if upper is not None and upper > 0:
        raw_ratio = current / upper
    final_bar_color = (255, 50, 50, 255)
    if raw_ratio >= 1.0:
        final_bar_color = (100, 255, 100, 255)
    elif raw_ratio > 0.8:
        final_bar_color = (255, 255, 100, 255)
    elif raw_ratio > 0.6:
        final_bar_color = (255, 200, 100, 255)
    elif raw_ratio > 0.4:
        final_bar_color = (255, 150, 100, 255)
    elif raw_ratio > 0.2:
        final_bar_color = (255, 100, 100, 255)

    with Frame().set_w(bar_width).set_h(18).set_content_align('lt'):
        progress = max(0.0, min(ratio, 1.0))
        total_w, total_h, border = bar_width, 14, 2
        progress_w = int((total_w - border * 2) * progress)
        progress_h = total_h - border * 2

        if progress > 0:
            Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 255), radius=total_h // 2))
            Spacer(w=progress_w, h=progress_h).set_bg(
                RoundRectBg(fill=final_bar_color, radius=(total_h - border) // 2)
            ).set_offset((border, border))

            for i in range(1, 5):
                lx = int((total_w - border * 2) * (i / 5.0))
                line_color = (100, 100, 100, 255) if i / 5.0 < progress else (150, 150, 150, 255)
                Spacer(w=1, h=total_h // 2 - 1).set_bg(FillBg(line_color)).set_offset((border + lx - 1, total_h // 2))
        else:
            Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 100), radius=total_h // 2))

    upper_text = "∞" if upper is None else f"{upper:,}"
    pct_text = _calc_mission_percent(current, upper)
    with HSplit().set_content_align('c').set_item_align('c').set_sep(8):
        TextBox(f"{current:,}/{upper_text} ({pct_text})", style_text).set_content_align('l')
        if next_need is not None:
            exp_text = "?" if next_exp is None else str(next_exp)
            TextBox(
                f"下一档{current:,}/{next_need:,} EXP+{exp_text}",
                TextStyle(font=DEFAULT_FONT, size=14, color=(80, 80, 80, 255)),
            ).set_content_align('r')
        else:
            TextBox(
                "下一档已满",
                TextStyle(font=DEFAULT_FONT, size=14, color=(80, 80, 80, 255)),
            ).set_content_align('r')


def _build_single_mission_card(
    title: str,
    current: int,
    upper: int | None,
    ratio: float,
    card_w: int,
    bar_color: Color = (82, 165, 255, 255),
    next_need: int | None = None,
    next_exp: int | None = None,
) -> Frame:
    with Frame().set_w(card_w).set_bg(roundrect_bg(fill=(255, 255, 255, 140))).set_padding((12, 10)) as card:
        with VSplit().set_content_align('l').set_item_align('l').set_sep(10):
            _draw_single_progress(
                title,
                current,
                upper,
                ratio,
                bar_width=card_w - 24,
                bar_color=bar_color,
                title_size=20,
                title_align='c',
                next_need=next_need,
                next_exp=next_exp,
            )
    return card


def _build_dual_mission_card(
    title: str,
    normal_current: int,
    normal_upper: int | None,
    normal_ratio: float,
    normal_next_need: int | None,
    normal_next_exp: int | None,
    ex_current: int,
    ex_upper: int | None,
    ex_ratio: float,
    ex_next_need: int | None,
    ex_next_exp: int | None,
    card_w: int,
    ex_round_text: str,
) -> Frame:
    with Frame().set_w(card_w).set_bg(roundrect_bg(fill=(255, 255, 255, 155))).set_padding((12, 10)) as card:
        with VSplit().set_content_align('l').set_item_align('l').set_sep(10):
            with Frame().set_w(card_w - 24).set_content_align('c'):
                with HSplit().set_content_align('c').set_item_align('c').set_sep(8):
                    TextBox(title, TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=(20, 20, 20, 255))).set_content_align('c')
            _draw_single_progress(
                "普通任务",
                normal_current,
                normal_upper,
                normal_ratio,
                bar_width=card_w - 24,
                bar_color=(84, 170, 255, 255),
                title_align='c',
                next_need=normal_next_need,
                next_exp=normal_next_exp,
            )
            _draw_single_progress(
                "EX任务",
                ex_current,
                ex_upper,
                ex_ratio,
                bar_width=card_w - 24,
                bar_color=(255, 145, 84, 255),
                title_align='c',
                title_badge=ex_round_text,
                next_need=ex_next_need,
                next_exp=ex_next_exp,
            )
    return card


async def compose_cr_test_character_rank_mission_overview_image(
    ctx: SekaiHandlerContext,
    profile: dict,
    err_msg: str,
    cid: int,
) -> Image.Image:
    async def get_masterdata_with_local_fallback(name: str):
        try:
            return await ctx.md.get(name)
        except Exception as e:
            local_path = pjoin(MASTER_DB_CACHE_DIR, ctx.region, f"{name}.json")
            if os.path.exists(local_path):
                logger.warning(
                    f"获取 MasterData [{ctx.region}.{name}] 失败，回退到本地文件: {get_exc_desc(e)}"
                )
                return load_json(local_path)
            raise e

    master_missions = await get_masterdata_with_local_fallback("characterMissionV2s")
    parameter_groups = await get_masterdata_with_local_fallback("characterMissionV2ParameterGroups")

    pg_seq_requirements: dict[int, list[tuple[int, int]]] = {}
    pg_seq_req_exp: dict[int, list[tuple[int, int, int]]] = {}
    pg_max_requirement: dict[int, int] = {}
    pg_seq_exp: dict[tuple[int, int], int] = {}
    for item in parameter_groups:
        pgid = item["id"]
        pg_seq_requirements.setdefault(pgid, []).append((item["seq"], item["requirement"]))
        pg_seq_req_exp.setdefault(pgid, []).append((item["seq"], item["requirement"], int(item.get("exp", 0))))
        pg_max_requirement[pgid] = max(pg_max_requirement.get(pgid, 0), item["requirement"])
        pg_seq_exp[(pgid, item["seq"])] = int(item.get("exp", 0))
    for items in pg_seq_requirements.values():
        items.sort(key=lambda x: x[0])
    for items in pg_seq_req_exp.values():
        items.sort(key=lambda x: x[0])

    def get_ex_round_requirement(pgid: int, round_no: int) -> int:
        req = 0
        for seq, requirement in pg_seq_requirements.get(pgid, []):
            if seq > round_no:
                break
            req = requirement
        return req

    def get_ex_round_exp(pgid: int, round_no: int) -> int:
        exp = 0
        for seq, _, seq_exp in pg_seq_req_exp.get(pgid, []):
            if seq > round_no:
                break
            exp = seq_exp
        return exp

    def calc_ex_round_and_progress(total: int, pgid: int) -> tuple[int, int, int]:
        total = max(0, int(total))
        round_no = 1
        while True:
            req = get_ex_round_requirement(pgid, round_no)
            if req <= 0 or total < req:
                return round_no, total, req
            total -= req
            round_no += 1

    def calc_ex_exp_limit_30_rounds(pgid: int) -> int:
        return sum(get_ex_round_requirement(pgid, i) for i in range(1, 31))

    char_missions = [m for m in master_missions if m.get("characterId") == cid]
    char_missions.sort(key=lambda x: x["id"])
    assert_and_reply(char_missions, f"找不到角色ID={cid}的任务数据")

    chara = await ctx.md.game_characters.find_by_id(cid)
    chara_name = (
        f"{chara.get('firstName', '')}{chara.get('givenName', '')}"
        if chara else (get_character_first_nickname(cid) or str(cid))
    )

    user_v2s = [item for item in (profile.get("userCharacterMissionV2s", []) or []) if item.get("characterId") == cid]
    user_statuses = [item for item in (profile.get("userCharacterMissionV2Statuses", []) or []) if item.get("characterId") == cid]
    user_items = [*user_v2s, *user_statuses]

    char_levels = await ctx.md.levels.find_by("levelType", "character", mode="all")
    char_levels = sorted(char_levels, key=lambda x: x["level"])
    char_level_total_exp = {int(x["level"]): int(x["totalExp"]) for x in char_levels}

    user_char = find_by(profile.get("userCharacters", []), "characterId", cid)
    assert_and_reply(user_char, "你的Suite数据来源没有提供userCharacters数据")
    cur_lv = int(user_char.get("characterRank", 1))
    cur_total_exp = int(user_char.get("totalExp", 0))
    if user_char.get("exp") is not None:
        cur_exp = int(user_char.get("exp", 0))
    else:
        cur_exp = max(0, cur_total_exp - char_level_total_exp.get(cur_lv, 0))

    pending_exp = 0
    for s in user_statuses:
        if s.get("missionStatus") != "achieved":
            continue
        pgid = int(s.get("parameterGroupId", 0))
        seq = int(s.get("seq", 0))
        pending_exp += pg_seq_exp.get((pgid, seq), 0)

    final_total_exp = cur_total_exp + pending_exp
    final_lv = 1
    final_lv_total = 0
    for lv_item in char_levels:
        if lv_item["totalExp"] <= final_total_exp:
            final_lv = int(lv_item["level"])
            final_lv_total = int(lv_item["totalExp"])
        else:
            break
    final_exp = final_total_exp - final_lv_total

    user_by_mission: dict[int, dict[str, int]] = {}
    user_by_type_progress: dict[str, int] = {}
    for item in user_items:
        mission_id = item.get("missionId")
        if mission_id is not None:
            cur = user_by_mission.setdefault(int(mission_id), {"progress": 0, "seq": 0})
            if item.get("progress") is not None:
                cur["progress"] = max(cur["progress"], int(item["progress"]))
            if item.get("seq") is not None:
                cur["seq"] = max(cur["seq"], int(item["seq"]))

        mission_type = item.get("characterMissionType")
        progress = item.get("progress")
        if mission_type and progress is not None:
            user_by_type_progress[mission_type] = max(user_by_type_progress.get(mission_type, 0), int(progress))

    ex_received_max_seq: dict[int, int] = {}
    for item in user_statuses:
        mission_id = item.get("missionId")
        seq = item.get("seq")
        if mission_id is None or seq is None:
            continue
        ex_received_max_seq[int(mission_id)] = max(ex_received_max_seq.get(int(mission_id), 0), int(seq))

    def get_ex_cleared_total(pgid: int, max_seq: int) -> int:
        if max_seq <= 0:
            return 0
        return sum(get_ex_round_requirement(pgid, i) for i in range(1, max_seq + 1))

    mission_rows = []
    for mission in char_missions:
        mission_id = int(mission["id"])
        mission_type = mission["characterMissionType"]
        pgid = int(mission["parameterGroupId"])
        is_ex = mission_type in CHAR_MISSION_EX_TYPES

        current = 0
        if is_ex:
            progress_raw = user_by_type_progress.get(mission_type, 0)
            received_seq = ex_received_max_seq.get(mission_id, 0)
            cleared_total = get_ex_cleared_total(pgid, received_seq)
            if progress_raw > 0:
                if progress_raw < cleared_total:
                    current = cleared_total + progress_raw
                else:
                    current = progress_raw
            else:
                current = cleared_total
        else:
            if mission_type in user_by_type_progress:
                current = user_by_type_progress[mission_type]
            elif mission_id in user_by_mission and user_by_mission[mission_id]["progress"] > 0:
                current = user_by_mission[mission_id]["progress"]
            elif mission_id in user_by_mission and user_by_mission[mission_id]["seq"] > 0:
                current = _get_pg_requirement_by_seq(pg_seq_requirements, pgid, user_by_mission[mission_id]["seq"])

        finite_upper = pg_max_requirement.get(pgid, 0)
        upper = None if is_ex else finite_upper
        ratio_upper = finite_upper if finite_upper > 0 else max(current, 1)
        ratio = 0.0 if ratio_upper <= 0 else min(current / ratio_upper, 1.0)
        next_need = None
        next_exp = None
        if is_ex:
            round_no, in_round_progress, round_need = calc_ex_round_and_progress(current, pgid)
            if round_need > 0:
                next_need = current + max(round_need - in_round_progress, 0)
                next_exp = get_ex_round_exp(pgid, round_no)
        else:
            for _, req, seq_exp in pg_seq_req_exp.get(pgid, []):
                if req > current:
                    next_need = req
                    next_exp = seq_exp
                    break

        mission_rows.append({
            "mission_id": mission_id,
            "mission_type": mission_type,
            "title": _char_mission_short_name(mission_type),
            "is_achievement": bool(mission.get("isAchievementMission", False)),
            "is_ex": is_ex,
            "current": current,
            "upper": upper,
            "ratio": ratio,
            "next_need": next_need,
            "next_exp": next_exp,
        })

    by_type = {item["mission_type"]: item for item in mission_rows}

    basic_rows = [item for item in mission_rows if not item["is_achievement"]]
    basic_order = [
        "collect_member",
        "collect_stamp",
        "collect_costume_3d",
        "collect_character_archive_voice",
        "collect_another_vocal",
        "read_mysekai_fixture_unique_character_talk",
        "read_area_talk",
    ]
    basic_order_idx = {name: i for i, name in enumerate(basic_order)}
    basic_rows.sort(key=lambda x: (basic_order_idx.get(x["mission_type"], 10 ** 9), x["mission_id"]))
    ach_rows = [
        item for item in mission_rows
        if item["is_achievement"]
        and item["mission_type"] not in CHAR_MISSION_EX_TYPES
        and item["mission_type"] not in CHAR_MISSION_EX_BASE_TYPES
    ]

    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(25, 25, 25, 255))
    sub_header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=(35, 35, 35, 255))
    card_w = 520
    card_sep = 16

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)

            with VSplit().set_content_align('l').set_item_align('l').set_sep(8).set_item_bg(roundrect_bg()):
                TextBox(
                    "各任务上限为MasterData中所规定的上限，并不一定是当前已实装资源总数",
                    TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(0, 0, 0)),
                    use_real_line_count=True,
                ).set_padding(12)

            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(12).set_content_align('lt').set_item_align('lt'):
                with HSplit().set_content_align('c').set_item_align('c').set_sep(12):
                    ImageBox(get_chara_icon_by_chara_id(cid), size=(48, 48))
                    TextBox(
                        f"{chara_name} 当前Lv.{cur_lv} EXP×{cur_exp} + 未领取EXP×{pending_exp} = 总计Lv.{final_lv} EXP×{final_exp}",
                        header_style,
                        use_real_line_count=True,
                    )

            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(12).set_content_align('lt').set_item_align('lt'):
                TextBox("基本任务", sub_header_style)
                for i in range(0, len(basic_rows), 2):
                    left = basic_rows[i]
                    right = basic_rows[i + 1] if i + 1 < len(basic_rows) else None
                    with HSplit().set_content_align('lt').set_item_align('lt').set_sep(card_sep):
                        _build_single_mission_card(
                            left["title"], left["current"], left["upper"], left["ratio"], card_w,
                            next_need=left.get("next_need"), next_exp=left.get("next_exp"),
                        )
                        if right:
                            _build_single_mission_card(
                                right["title"], right["current"], right["upper"], right["ratio"], card_w,
                                next_need=right.get("next_need"), next_exp=right.get("next_exp"),
                            )
                        else:
                            Spacer(w=card_w, h=1)

            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(12).set_content_align('lt').set_item_align('lt'):
                TextBox("成就", sub_header_style)

                play_live = by_type.get("play_live", {"current": 0, "upper": 0, "ratio": 0})
                play_live_ex = by_type.get("play_live_ex", {"current": 0, "upper": None, "ratio": 0})
                waiting_room = by_type.get("waiting_room", {"current": 0, "upper": 0, "ratio": 0})
                waiting_room_ex = by_type.get("waiting_room_ex", {"current": 0, "upper": None, "ratio": 0})

                play_live_ex_total = play_live_ex["current"]
                waiting_room_ex_total = waiting_room_ex["current"]
                play_live_ex_limit = calc_ex_exp_limit_30_rounds(101)
                waiting_room_ex_limit = calc_ex_exp_limit_30_rounds(102)
                play_live_ex_ratio = min(play_live_ex_total / max(play_live_ex_limit, 1), 1.0)
                waiting_room_ex_ratio = min(waiting_room_ex_total / max(waiting_room_ex_limit, 1), 1.0)

                play_live_round, _, _ = calc_ex_round_and_progress(play_live_ex_total, 101)
                waiting_room_round, _, _ = calc_ex_round_and_progress(waiting_room_ex_total, 102)

                with HSplit().set_content_align('lt').set_item_align('lt').set_sep(card_sep):
                    _build_dual_mission_card(
                        "队长次数",
                        play_live["current"], play_live["upper"], play_live["ratio"],
                        play_live.get("next_need"), play_live.get("next_exp"),
                        play_live_ex_total, play_live_ex_limit, play_live_ex_ratio,
                        play_live_ex.get("next_need"), play_live_ex.get("next_exp"),
                        card_w,
                        f"EX {play_live_round} 回目",
                    )
                    _build_dual_mission_card(
                        "休息室次数",
                        waiting_room["current"], waiting_room["upper"], waiting_room["ratio"],
                        waiting_room.get("next_need"), waiting_room.get("next_exp"),
                        waiting_room_ex_total, waiting_room_ex_limit, waiting_room_ex_ratio,
                        waiting_room_ex.get("next_need"), waiting_room_ex.get("next_exp"),
                        card_w,
                        f"EX {waiting_room_round} 回目",
                    )

                for i in range(0, len(ach_rows), 2):
                    left = ach_rows[i]
                    right = ach_rows[i + 1] if i + 1 < len(ach_rows) else None
                    with HSplit().set_content_align('lt').set_item_align('lt').set_sep(card_sep):
                        _build_single_mission_card(
                            left["title"], left["current"], left["upper"], left["ratio"], card_w,
                            next_need=left.get("next_need"), next_exp=left.get("next_exp"),
                        )
                        if right:
                            _build_single_mission_card(
                                right["title"], right["current"], right["upper"], right["ratio"], card_w,
                                next_need=right.get("next_need"), next_exp=right.get("next_exp"),
                            )
                        else:
                            Spacer(w=card_w, h=1)

    add_watermark(canvas)
    return await canvas.get_img()


async def compose_cr_test_character_rank_mission_all_image(
    ctx: SekaiHandlerContext,
    qid: int,
    cid: int,
    mission_type: str,
) -> Image.Image:
    async def get_masterdata_with_local_fallback(name: str):
        try:
            return await ctx.md.get(name)
        except Exception as e:
            local_path = pjoin(MASTER_DB_CACHE_DIR, ctx.region, f"{name}.json")
            if os.path.exists(local_path):
                logger.warning(f"获取 MasterData [{ctx.region}.{name}] 失败，回退到本地文件: {get_exc_desc(e)}")
                return load_json(local_path)
            raise e

    profile, err_msg = await get_detailed_profile(
        ctx,
        qid,
        filter=get_detailed_profile_card_filter("userCharacterMissionV2s", "userCharacterMissionV2Statuses", "userCharacters"),
        raise_exc=True,
    )

    master_missions = await get_masterdata_with_local_fallback("characterMissionV2s")
    parameter_groups = await get_masterdata_with_local_fallback("characterMissionV2ParameterGroups")

    user_v2s = [item for item in (profile.get("userCharacterMissionV2s", []) or []) if item.get("characterId") == cid]
    user_statuses = [item for item in (profile.get("userCharacterMissionV2Statuses", []) or []) if item.get("characterId") == cid]

    chara = await ctx.md.game_characters.find_by_id(cid)
    chara_name = f"{chara.get('firstName', '')}{chara.get('givenName', '')}" if chara else (get_character_first_nickname(cid) or str(cid))

    def build_section(target_mission_type: str) -> dict:
        mission = find_by_predicate(
            [m for m in master_missions if int(m.get("characterId", 0)) == cid],
            lambda x: x.get("characterMissionType") == target_mission_type,
        )
        assert_and_reply(mission is not None, f"找不到该角色的任务类型: {target_mission_type}")

        pgid = int(mission["parameterGroupId"])
        group_rows = sorted(
            [item for item in parameter_groups if int(item.get("id", 0)) == pgid],
            key=lambda x: int(x["seq"]),
        )
        assert_and_reply(group_rows, f"找不到任务参数组: {pgid}")

        pg_seq_requirements = [(int(item["seq"]), int(item["requirement"])) for item in group_rows]
        pg_seq_req_exp = [(int(item["seq"]), int(item["requirement"]), int(item.get("exp", 0))) for item in group_rows]

        def get_ex_round_requirement(round_no: int) -> int:
            req = 0
            for seq, requirement in pg_seq_requirements:
                if seq > round_no:
                    break
                req = requirement
            return req

        def get_ex_round_exp(round_no: int) -> int:
            exp = 0
            for seq, _, seq_exp in pg_seq_req_exp:
                if seq > round_no:
                    break
                exp = seq_exp
            return exp

        def calc_ex_round_and_progress(total: int) -> tuple[int, int, int]:
            total = max(0, int(total))
            round_no = 1
            while True:
                req = get_ex_round_requirement(round_no)
                if req <= 0 or total < req:
                    return round_no, total, req
                total -= req
                round_no += 1

        def calc_ex_exp_limit_30_rounds() -> int:
            return sum(get_ex_round_requirement(i) for i in range(1, 31))

        progress_raw = 0
        for item in user_v2s:
            if item.get("characterMissionType") == target_mission_type and item.get("progress") is not None:
                progress_raw = max(progress_raw, int(item["progress"]))

        is_ex = target_mission_type.endswith("_ex")
        current_total = progress_raw
        if is_ex:
            completed_seq_list: list[int] = []
            for item in user_statuses:
                if int(item.get("parameterGroupId", 0)) != pgid:
                    continue
                if item.get("seq") is None:
                    continue
                completed_seq_list.append(int(item["seq"]))
            received_seq = max(completed_seq_list) if completed_seq_list else 0
            cleared_total = sum(get_ex_round_requirement(i) for i in range(1, received_seq + 1))
            if progress_raw < cleared_total:
                current_total = cleared_total + progress_raw
            elif progress_raw == 0:
                current_total = cleared_total

        reached_seq = 0
        current_round_no = None
        current_round_progress = None
        current_round_need = None
        next_need = None
        next_exp = None
        acc_req = 0
        acc_exp = 0
        display_rows = []
        if is_ex:
            current_round_no, current_round_progress, current_round_need = calc_ex_round_and_progress(current_total)
            reached_seq = current_round_no
            max_round = max(
                current_round_no,
                max((int(item.get("seq", 0)) for item in user_statuses if int(item.get("parameterGroupId", 0)) == pgid), default=0),
                max((int(item["seq"]) for item in group_rows), default=0),
            )
            for round_no in range(1, max_round + 1):
                req = get_ex_round_requirement(round_no)
                exp = get_ex_round_exp(round_no)
                acc_req += req
                acc_exp += exp
                display_rows.append({
                    "seq": round_no,
                    "requirement": req,
                    "acc_requirement": acc_req,
                    "exp": exp,
                    "acc_exp": acc_exp,
                })
            if current_round_need and current_round_need > 0 and current_round_progress is not None:
                next_need = current_total + max(current_round_need - current_round_progress, 0)
                next_exp = get_ex_round_exp(current_round_no)
        else:
            for row in group_rows:
                seq = int(row["seq"])
                req = int(row["requirement"])
                exp = int(row.get("exp", 0))
                acc_req = req
                acc_exp += exp
                if current_total >= req:
                    reached_seq = seq
                elif next_need is None:
                    next_need = req
                    next_exp = exp
                display_rows.append({
                    "seq": seq,
                    "requirement": req,
                    "acc_requirement": req,
                    "exp": exp,
                    "acc_exp": acc_exp,
                })

        return {
            "mission_type": target_mission_type,
            "title": CHAR_MISSION_SHORT_NAMES.get(target_mission_type, target_mission_type),
            "is_ex": is_ex,
            "current_total": current_total,
            "reached_seq": reached_seq,
            "current_round_no": current_round_no,
            "current_round_progress": current_round_progress,
            "current_round_need": current_round_need,
            "upper": calc_ex_exp_limit_30_rounds() if is_ex else (max((int(row["requirement"]) for row in group_rows), default=0)),
            "ratio": min(current_total / max(calc_ex_exp_limit_30_rounds(), 1), 1.0) if is_ex else (
                min(current_total / max((max((int(row["requirement"]) for row in group_rows), default=1)), 1), 1.0)
            ),
            "next_need": next_need,
            "next_exp": next_exp,
            "display_rows": display_rows,
        }

    section_types = [mission_type]
    if mission_type == "play_live":
        section_types = ["play_live", "play_live_ex"]
    elif mission_type == "waiting_room":
        section_types = ["waiting_room", "waiting_room_ex"]
    sections = [build_section(mt) for mt in section_types]

    title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=26, color=BLACK)
    style1 = TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=BLACK)
    style2 = TextStyle(font=DEFAULT_FONT, size=20, color=(50, 50, 50))
    style3 = TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=(200, 50, 50))
    sub_header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=(35, 35, 35, 255))

    gh, vsep, hsep = 40, 6, 6
    chunk_size_limit = 40
    gw_seq, gw_req, gw_acc_req, gw_exp, gw_acc_exp = 84, 96, 128, 72, 116

    def draw_section_table(section: dict, target_col_count: int | None = None):
        def bg_fn(i: int, w: Widget):
            row_seq = w.userdata.get("row_seq")
            if row_seq == section["reached_seq"] and section["reached_seq"] > 0:
                return FillBg((255, 244, 196, 210))
            return FillBg((255, 255, 255, 200)) if i % 2 == 0 else FillBg((255, 255, 255, 100))

        rows = section["display_rows"]
        if target_col_count and target_col_count > 1:
            chunk_size = max(1, math.ceil(len(rows) / target_col_count))
        else:
            chunk_size = chunk_size_limit
        chunks = [rows[i:i + chunk_size] for i in range(0, len(rows), chunk_size)] or [[]]
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_padding(8).set_bg(roundrect_bg(fill=(255, 255, 255, 120))):
            TextBox("EX任务" if section["is_ex"] else "普通任务", sub_header_style)
            with HSplit().set_content_align('lb').set_item_align('lb').set_sep(8):
                TextBox("当前进度:", style1)
                TextBox(str(section["current_total"]), style3)
                if section["is_ex"] and section["current_round_no"] is not None:
                    TextBox(f"当前回目 EX {section['current_round_no']}", style2)
                elif section["reached_seq"] > 0:
                    TextBox(f"已达档位 #{section['reached_seq']}", style2)

            _draw_single_progress(
                "",
                section["current_total"],
                section["upper"],
                section["ratio"],
                bar_width=560,
                bar_color=(255, 145, 84, 255) if section["is_ex"] else (84, 170, 255, 255),
                title_align='l',
                title_badge=None,
                next_need=section.get("next_need"),
                next_exp=section.get("next_exp"),
            )

            with HSplit().set_content_align('lt').set_item_align('lt').set_sep(12):
                for chunk in chunks:
                    with HSplit().set_content_align('lt').set_item_align('lt').set_sep(hsep):
                        with VSplit().set_content_align('c').set_item_align('c').set_sep(vsep).set_item_bg(bg_fn):
                            TextBox("档位", style1).set_size((gw_seq, gh)).set_content_align('c')
                            for row in chunk:
                                w = TextBox(f"#{row['seq']}", style2).set_size((gw_seq, gh)).set_content_align('c')
                                w.userdata["row_seq"] = row["seq"]

                        with VSplit().set_content_align('c').set_item_align('c').set_sep(vsep).set_item_bg(bg_fn):
                            TextBox("需求", style1).set_size((gw_req, gh)).set_content_align('c')
                            for row in chunk:
                                w = TextBox(str(row["requirement"]), style2).set_size((gw_req, gh)).set_content_align('c')
                                w.userdata["row_seq"] = row["seq"]

                        with VSplit().set_content_align('c').set_item_align('c').set_sep(vsep).set_item_bg(bg_fn):
                            TextBox("累计需求", style1).set_size((gw_acc_req, gh)).set_content_align('c')
                            for row in chunk:
                                w = TextBox(str(row["acc_requirement"]), style2).set_size((gw_acc_req, gh)).set_content_align('c')
                                w.userdata["row_seq"] = row["seq"]

                        with VSplit().set_content_align('c').set_item_align('c').set_sep(vsep).set_item_bg(bg_fn):
                            TextBox("EXP", style1).set_size((gw_exp, gh)).set_content_align('c')
                            for row in chunk:
                                w = TextBox(str(row["exp"]), style2).set_size((gw_exp, gh)).set_content_align('c')
                                w.userdata["row_seq"] = row["seq"]

                        with VSplit().set_content_align('c').set_item_align('c').set_sep(vsep).set_item_bg(bg_fn):
                            TextBox("累计EXP", style1).set_size((gw_acc_exp, gh)).set_content_align('c')
                            for row in chunk:
                                w = TextBox(str(row["acc_exp"]), style2).set_size((gw_acc_exp, gh)).set_content_align('c')
                                w.userdata["row_seq"] = row["seq"]

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_item_bg(roundrect_bg()):
            await get_detailed_profile_card(ctx, profile, err_msg)

            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(8).set_padding(8):
                with HSplit().set_content_align('lb').set_item_align('c').set_sep(8):
                    ImageBox(get_chara_icon_by_chara_id(cid), size=(48, 48))
                    title = sections[0]["title"]
                    if len(sections) == 2:
                        title = CHAR_MISSION_SHORT_NAMES.get(mission_type, mission_type)
                    TextBox(f"{chara_name} {title} 任务详览", title_style)
                TextBox("普通任务高亮栏为已达成的最近档位，EX任务高亮栏为当前进行中的档位", style2)

            normal_section = find_by_predicate(sections, lambda x: not x["is_ex"])
            normal_col_count = None
            if normal_section:
                normal_col_count = max(1, math.ceil(len(normal_section["display_rows"]) / chunk_size_limit))

            for section in sections:
                section_col_count = None
                if section["is_ex"] and normal_col_count:
                    section_col_count = normal_col_count
                draw_section_table(section, section_col_count)

    add_watermark(canvas)
    return await canvas.get_img()


cr_test_character_rank_mission = SekaiCmdHandler([
    "/cr任务", "/crtest", "/pjsk cr test",
])
cr_test_character_rank_mission.check_cdrate(cd).check_wblist(gbl)


@cr_test_character_rank_mission.handle()
async def _(ctx: SekaiHandlerContext):
    help_msg = f"""
使用方式:
1. {ctx.original_trigger_cmd} 角色名
2. {ctx.original_trigger_cmd} 角色名 all 任务名
示例:
{ctx.original_trigger_cmd} miku
{ctx.original_trigger_cmd} miku all 队长次数
发送“{ctx.original_trigger_cmd} help”获取详细帮助
""".strip()
    raw_args = ctx.get_args().strip()
    assert_and_reply(raw_args, help_msg)
    if raw_args.lower() in ("help", "帮助"):
        help_text = f"""
# CR任务（测试入口）

查询指定角色的CR任务进度，或查看某个任务的全量档位表。
需要📡抓包数据。
支持服务器: `所有`

## 基础用法

- `{ctx.original_trigger_cmd} miku`
- `{ctx.original_trigger_cmd} miku all 队长次数`

## 查询模式

- `角色名`
  查询该角色的角色任务总览
- `角色名 all 任务名`
  查询该任务的全量档位、累计需求和累计EXP

## 说明

- 该命令是独立测试入口，不占用现有 `/角色等级任务`
- `队长次数` 和 `休息室次数` 在 `all` 视图下会同时显示普通任务和EX任务
- 其他任务在 `all` 视图下只显示对应单个任务表

## 可用任务名示例

- `队长次数` `队长`
- `休息室次数` `休息室` `控制室`
- `服装` `衣装`
- `表情` `贴纸`
- `区域对话`
- `前篇` `前编`
- `后篇` `后编`
- `anvo`
- `单人家具` `单人道具`
- `团家具`
- `树花` `属性家具` `属性道具` `植物`
- `卡面` `图鉴` `成员`
- `4星技能` `四星技能` `四星slv` `4星slv`
- `低星技能` `低星slv`
- `4星专精` `四星专精` `四星突破` `4星突破` `4星mr` `四星mr`
- `低星专精` `低星突破` `低星mr`
- `台词` `语音`
- `ms家具` `烤森家具`
- `ms画布` `烤森画布`
- `ms对话` `烤森对话`
""".strip()
        return await ctx.asend_reply_msg(await get_image_cq(
            await markdown_to_image(help_text, width=760),
            low_quality=True,
        ))

    nickname, rest = extract_nickname_from_args(raw_args)
    assert_and_reply(nickname, f"未识别到角色名称\n{help_msg}")
    cid = get_cid_by_nickname(nickname)
    assert_and_reply(cid is not None, f"角色名无效: {nickname}")

    rest = rest.strip()
    if rest:
        show_all, rest = extract_cr_test_all_flag(rest)
        if show_all:
            mission_type, rest = extract_cr_test_mission_type(rest)
            assert_and_reply(mission_type is not None and not rest.strip(), f"未识别到角色等级任务名\n{help_msg}")
            return await ctx.asend_reply_msg(await get_image_cq(
                await compose_cr_test_character_rank_mission_all_image(ctx, ctx.user_id, cid, mission_type),
                low_quality=True,
            ))
        assert_and_reply(False, f"参数无法解析: {rest}\n{help_msg}")

    profile, err_msg = await get_detailed_profile(
        ctx,
        ctx.user_id,
        filter=get_detailed_profile_card_filter("userCharacterMissionV2s", "userCharacterMissionV2Statuses", "userCharacters"),
        raise_exc=True,
    )

    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_cr_test_character_rank_mission_overview_image(ctx, profile, err_msg, cid),
        low_quality=True,
    ))

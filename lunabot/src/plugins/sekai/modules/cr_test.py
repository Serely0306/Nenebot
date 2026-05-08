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


CR_TEST_MISSION_SHORT_NAMES = {
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
    "area_item_level_up_reality_world": "属性道具升级次数",
    "collect_member": "卡面",
    "skill_level_up_rare": "技能等级升级次数(4星/生日)",
    "skill_level_up_standard": "技能等级升级次数(低星)",
    "master_rank_up_rare": "专精等级升级次数(4星/生日)",
    "master_rank_up_standard": "专精等级升级次数(低星)",
    "collect_character_archive_voice": "台词",
    "collect_mysekai_fixture": "MySekai家具数量",
    "collect_mysekai_canvas": "MySekai画布数量",
    "read_mysekai_fixture_unique_character_talk": "MySekai对话",
}

CR_TEST_ALL_KEYWORDS = ("all", "全部", "全量", "总表", "表格")
CR_TEST_EX_TYPES = {"play_live_ex", "waiting_room_ex"}
CR_TEST_EX_BASE_TYPES = {"play_live", "waiting_room"}
CR_TEST_MISSION_TYPE_ALIASES = {
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
    "area_item_level_up_reality_world": ["属性道具升级次数", "树花", "属性家具", "属性道具", "植物"],
    "collect_member": ["卡面", "图鉴", "成员"],
    "skill_level_up_rare": ["技能等级升级次数(4星/生日)", "4星技能", "四星技能", "四星slv", "4星slv"],
    "skill_level_up_standard": ["技能等级升级次数(低星)", "低星技能", "低星slv"],
    "master_rank_up_rare": ["专精等级升级次数(4星/生日)", "4星专精", "四星专精", "四星突破", "4星突破", "4星mr", "四星mr"],
    "master_rank_up_standard": ["专精等级升级次数(低星)", "低星专精", "低星突破", "低星mr"],
    "collect_character_archive_voice": ["台词", "语音"],
    "collect_mysekai_fixture": ["mysekai家具数量", "ms家具", "烤森家具"],
    "collect_mysekai_canvas": ["mysekai画布数量", "ms画布", "烤森画布"],
    "read_mysekai_fixture_unique_character_talk": ["mysekai对话", "ms对话", "烤森对话"],
}
for _mission_type, _short_name in CR_TEST_MISSION_SHORT_NAMES.items():
    _normalized = _short_name.lower().replace(" ", "").replace("（", "(").replace("）", ")")
    CR_TEST_MISSION_TYPE_ALIASES.setdefault(_mission_type, []).append(_normalized)
    CR_TEST_MISSION_TYPE_ALIASES[_mission_type].append(_mission_type.lower())


def _cr_test_normalize_query(text: str) -> str:
    return text.strip().lower().replace(" ", "").replace("（", "(").replace("）", ")")


def extract_cr_test_all_flag(args: str) -> tuple[bool, str]:
    normalized = _cr_test_normalize_query(args)
    for keyword in CR_TEST_ALL_KEYWORDS:
        if keyword in normalized:
            return True, args.replace(keyword, "", 1).strip()
    return False, args.strip()


def extract_cr_test_mission_type(args: str) -> tuple[str | None, str]:
    normalized = _cr_test_normalize_query(args)
    pairs: list[tuple[str, str]] = []
    for mission_type, aliases in CR_TEST_MISSION_TYPE_ALIASES.items():
        for alias in aliases:
            pairs.append((mission_type, alias))
    pairs.sort(key=lambda x: len(x[1]), reverse=True)
    for mission_type, alias in pairs:
        if alias in normalized:
            return mission_type, ""
    return None, args.strip()


async def _cr_test_get_masterdata(ctx: SekaiHandlerContext, name: str):
    try:
        return await ctx.md.get(name)
    except Exception as e:
        local_path = pjoin(MASTER_DB_CACHE_DIR, ctx.region, f"{name}.json")
        if os.path.exists(local_path):
            logger.warning(f"获取 MasterData [{ctx.region}.{name}] 失败，回退到本地文件: {get_exc_desc(e)}")
            return load_json(local_path)
        raise e


def _cr_test_group_parameter_rows(parameter_groups: list[dict]) -> dict[int, list[dict]]:
    grouped: dict[int, list[dict]] = {}
    for row in parameter_groups:
        grouped.setdefault(int(row["id"]), []).append(row)
    for rows in grouped.values():
        rows.sort(key=lambda x: int(x["seq"]))
    return grouped


def _cr_test_requirement_by_seq(grouped_rows: dict[int, list[dict]], pgid: int, seq: int) -> int:
    req = 0
    for row in grouped_rows.get(pgid, []):
        if int(row["seq"]) > seq:
            break
        req = int(row["requirement"])
    return req


def _cr_test_exp_by_seq(grouped_rows: dict[int, list[dict]], pgid: int, seq: int) -> int:
    exp = 0
    for row in grouped_rows.get(pgid, []):
        if int(row["seq"]) > seq:
            break
        exp = int(row.get("exp", 0))
    return exp


def _cr_test_calc_ex_round(grouped_rows: dict[int, list[dict]], pgid: int, total: int) -> tuple[int, int, int]:
    total = max(0, int(total))
    round_no = 1
    while True:
        req = _cr_test_requirement_by_seq(grouped_rows, pgid, round_no)
        if req <= 0 or total < req:
            return round_no, total, req
        total -= req
        round_no += 1


def _cr_test_calc_ex_limit(grouped_rows: dict[int, list[dict]], pgid: int, round_count: int = 30) -> int:
    return sum(_cr_test_requirement_by_seq(grouped_rows, pgid, i) for i in range(1, round_count + 1))


def _cr_test_build_user_progress_maps(profile: dict, cid: int) -> tuple[dict[int, dict[str, int]], dict[str, int], dict[int, int]]:
    user_v2s = [item for item in (profile.get("userCharacterMissionV2s", []) or []) if item.get("characterId") == cid]
    user_statuses = [item for item in (profile.get("userCharacterMissionV2Statuses", []) or []) if item.get("characterId") == cid]

    user_by_mission: dict[int, dict[str, int]] = {}
    user_by_type_progress: dict[str, int] = {}
    for item in [*user_v2s, *user_statuses]:
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

    return user_by_mission, user_by_type_progress, ex_received_max_seq


def _cr_test_current_progress(
    mission: dict,
    grouped_rows: dict[int, list[dict]],
    user_by_mission: dict[int, dict[str, int]],
    user_by_type_progress: dict[str, int],
    ex_received_max_seq: dict[int, int],
) -> int:
    mission_id = int(mission["id"])
    mission_type = mission["characterMissionType"]
    pgid = int(mission["parameterGroupId"])
    is_ex = mission_type in CR_TEST_EX_TYPES

    if is_ex:
        progress_raw = user_by_type_progress.get(mission_type, 0)
        received_seq = ex_received_max_seq.get(mission_id, 0)
        cleared_total = sum(_cr_test_requirement_by_seq(grouped_rows, pgid, i) for i in range(1, received_seq + 1))
        if progress_raw > 0:
            if progress_raw < cleared_total:
                return cleared_total + progress_raw
            return progress_raw
        return cleared_total

    if mission_type in user_by_type_progress:
        return user_by_type_progress[mission_type]
    if mission_id in user_by_mission and user_by_mission[mission_id]["progress"] > 0:
        return user_by_mission[mission_id]["progress"]
    if mission_id in user_by_mission and user_by_mission[mission_id]["seq"] > 0:
        return _cr_test_requirement_by_seq(grouped_rows, pgid, user_by_mission[mission_id]["seq"])
    return 0


def _cr_test_next_progress(
    mission_type: str,
    pgid: int,
    current: int,
    grouped_rows: dict[int, list[dict]],
) -> tuple[int | None, int | None]:
    if mission_type in CR_TEST_EX_TYPES:
        round_no, in_round_progress, round_need = _cr_test_calc_ex_round(grouped_rows, pgid, current)
        if round_need <= 0:
            return None, None
        return current + max(round_need - in_round_progress, 0), _cr_test_exp_by_seq(grouped_rows, pgid, round_no)

    for row in grouped_rows.get(pgid, []):
        requirement = int(row["requirement"])
        if requirement > current:
            return requirement, int(row.get("exp", 0))
    return None, None


def _cr_test_mission_rows(
    profile: dict,
    cid: int,
    master_missions: list[dict],
    grouped_rows: dict[int, list[dict]],
) -> list[dict]:
    user_by_mission, user_by_type_progress, ex_received_max_seq = _cr_test_build_user_progress_maps(profile, cid)
    rows = []
    for mission in sorted([m for m in master_missions if int(m.get("characterId", 0)) == cid], key=lambda x: int(x["id"])):
        mission_type = mission["characterMissionType"]
        pgid = int(mission["parameterGroupId"])
        current = _cr_test_current_progress(
            mission,
            grouped_rows,
            user_by_mission,
            user_by_type_progress,
            ex_received_max_seq,
        )
        is_ex = mission_type in CR_TEST_EX_TYPES
        finite_upper = max([int(row["requirement"]) for row in grouped_rows.get(pgid, [])] or [0])
        upper = None if is_ex else finite_upper
        ratio_upper = _cr_test_calc_ex_limit(grouped_rows, pgid) if is_ex else finite_upper
        ratio = min(current / max(ratio_upper, 1), 1.0)
        next_need, next_exp = _cr_test_next_progress(mission_type, pgid, current, grouped_rows)
        rows.append({
            "mission": mission,
            "mission_id": int(mission["id"]),
            "mission_type": mission_type,
            "pgid": pgid,
            "title": CR_TEST_MISSION_SHORT_NAMES.get(mission_type, mission_type),
            "is_ex": is_ex,
            "is_achievement": bool(mission.get("isAchievementMission", False)),
            "current": current,
            "upper": upper,
            "ratio": ratio,
            "next_need": next_need,
            "next_exp": next_exp,
        })
    return rows


def _cr_test_format_progress(current: int, upper: int | None) -> str:
    if upper is None:
        return f"{current:,}/∞"
    return f"{current:,}/{upper:,}"


def _cr_test_progress_bar(current: int, upper: int | None, ratio: float, width: int, height: int = 16):
    with Frame().set_w(width).set_h(height).set_content_align('lt'):
        Spacer(w=width, h=height).set_bg(RoundRectBg(fill=(100, 100, 100, 100), radius=height // 2))
        progress_w = int(width * max(0.0, min(ratio, 1.0)))
        color = (255, 50, 50, 255)
        raw_ratio = current / max(upper or current or 1, 1)
        if raw_ratio >= 1.0 and upper is not None:
            color = (100, 255, 100, 255)
        elif raw_ratio > 0.8:
            color = (255, 255, 100, 255)
        elif raw_ratio > 0.6:
            color = (255, 200, 100, 255)
        elif raw_ratio > 0.4:
            color = (255, 150, 100, 255)
        elif raw_ratio > 0.2:
            color = (255, 100, 100, 255)
        if progress_w > 0:
            Spacer(w=progress_w, h=height).set_bg(RoundRectBg(fill=color, radius=height // 2))


def _cr_test_mission_card(row: dict, width: int = 500):
    with VSplit().set_w(width).set_content_align('l').set_item_align('l').set_sep(8).set_padding(12).set_bg(roundrect_bg(fill=(255, 255, 255, 160))):
        with HSplit().set_content_align('l').set_item_align('c').set_sep(8):
            TextBox(row["title"], TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=(30, 30, 30)))
            if row["is_ex"]:
                TextBox("EX", TextStyle(font=DEFAULT_FONT, size=16, color=(90, 90, 90)))
        _cr_test_progress_bar(row["current"], row["upper"], row["ratio"], width - 24)
        suffix = "下一档已满"
        if row["next_need"] is not None:
            suffix = f"下一档 {row['current']:,}/{row['next_need']:,} EXP+{row['next_exp'] or 0}"
        TextBox(
            f"{_cr_test_format_progress(row['current'], row['upper'])}  {suffix}",
            TextStyle(font=DEFAULT_FONT, size=16, color=(70, 70, 70)),
            use_real_line_count=True,
        ).set_w(width - 24)


async def compose_cr_test_character_rank_mission_overview_image(
    ctx: SekaiHandlerContext,
    profile: dict,
    err_msg: str,
    cid: int,
) -> Image.Image:
    master_missions = await _cr_test_get_masterdata(ctx, "characterMissionV2s")
    parameter_groups = await _cr_test_get_masterdata(ctx, "characterMissionV2ParameterGroups")
    grouped_rows = _cr_test_group_parameter_rows(parameter_groups)
    rows = _cr_test_mission_rows(profile, cid, master_missions, grouped_rows)
    assert_and_reply(rows, f"找不到角色ID={cid}的任务数据")

    chara = await ctx.md.game_characters.find_by_id(cid)
    chara_name = f"{chara.get('firstName', '')}{chara.get('givenName', '')}" if chara else (get_character_first_nickname(cid) or str(cid))
    user_char = find_by(profile.get("userCharacters", []), "characterId", cid)
    assert_and_reply(user_char, "你的Suite数据来源没有提供userCharacters数据")
    chara_rank = int(user_char.get("characterRank", 1))
    total_exp = int(user_char.get("totalExp", 0))

    basic_order = [
        "collect_member",
        "collect_stamp",
        "collect_costume_3d",
        "collect_character_archive_voice",
        "collect_another_vocal",
        "read_mysekai_fixture_unique_character_talk",
        "read_area_talk",
    ]
    order_idx = {name: i for i, name in enumerate(basic_order)}
    basic_rows = [row for row in rows if not row["is_achievement"]]
    basic_rows.sort(key=lambda x: (order_idx.get(x["mission_type"], 10**9), x["mission_id"]))
    ach_rows = [
        row for row in rows
        if row["is_achievement"]
        and row["mission_type"] not in CR_TEST_EX_TYPES
        and row["mission_type"] not in CR_TEST_EX_BASE_TYPES
    ]
    by_type = {row["mission_type"]: row for row in rows}

    card_w = 520
    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            await get_detailed_profile_card(ctx, profile, err_msg)
            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(10).set_content_align('lt').set_item_align('lt'):
                with HSplit().set_content_align('l').set_item_align('c').set_sep(10):
                    ImageBox(get_chara_icon_by_chara_id(cid), size=(48, 48))
                    TextBox(
                        f"{chara_name} Lv.{chara_rank} totalExp={total_exp}",
                        TextStyle(font=DEFAULT_BOLD_FONT, size=26, color=BLACK),
                    )

            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(12).set_content_align('lt').set_item_align('lt'):
                TextBox("基本任务", TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=BLACK))
                for i in range(0, len(basic_rows), 2):
                    with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                        _cr_test_mission_card(basic_rows[i], card_w)
                        if i + 1 < len(basic_rows):
                            _cr_test_mission_card(basic_rows[i + 1], card_w)
                        else:
                            Spacer(w=card_w, h=1)

            with VSplit().set_bg(roundrect_bg()).set_padding(16).set_sep(12).set_content_align('lt').set_item_align('lt'):
                TextBox("成就任务", TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=BLACK))
                pair_rows = []
                for base_type, ex_type, title in (
                    ("play_live", "play_live_ex", "队长次数"),
                    ("waiting_room", "waiting_room_ex", "休息室次数"),
                ):
                    if base_type in by_type:
                        base_row = by_type[base_type].copy()
                        base_row["title"] = title
                        pair_rows.append(base_row)
                    if ex_type in by_type:
                        pair_rows.append(by_type[ex_type])
                for row in pair_rows + ach_rows:
                    _cr_test_mission_card(row, card_w * 2 + 16)

    add_watermark(canvas)
    return await canvas.get_img()


async def compose_cr_test_character_rank_mission_all_image(
    ctx: SekaiHandlerContext,
    qid: int,
    cid: int,
    mission_type: str,
) -> Image.Image:
    profile, err_msg = await get_detailed_profile(
        ctx,
        qid,
        filter=get_detailed_profile_card_filter("userCharacterMissionV2s", "userCharacterMissionV2Statuses", "userCharacters"),
        raise_exc=True,
    )
    master_missions = await _cr_test_get_masterdata(ctx, "characterMissionV2s")
    parameter_groups = await _cr_test_get_masterdata(ctx, "characterMissionV2ParameterGroups")
    grouped_rows = _cr_test_group_parameter_rows(parameter_groups)
    rows = _cr_test_mission_rows(profile, cid, master_missions, grouped_rows)
    by_type = {row["mission_type"]: row for row in rows}

    section_types = [mission_type]
    if mission_type == "play_live":
        section_types = ["play_live", "play_live_ex"]
    elif mission_type == "waiting_room":
        section_types = ["waiting_room", "waiting_room_ex"]

    sections = []
    for mt in section_types:
        row = by_type.get(mt)
        assert_and_reply(row is not None, f"找不到该角色的任务类型: {mt}")
        pgid = row["pgid"]
        current = row["current"]
        acc_req = 0
        acc_exp = 0
        table_rows = []
        if mt in CR_TEST_EX_TYPES:
            current_round, _, _ = _cr_test_calc_ex_round(grouped_rows, pgid, current)
            max_round = max(current_round, max([int(item["seq"]) for item in grouped_rows.get(pgid, [])] or [1]))
            for seq in range(1, max_round + 1):
                req = _cr_test_requirement_by_seq(grouped_rows, pgid, seq)
                exp = _cr_test_exp_by_seq(grouped_rows, pgid, seq)
                acc_req += req
                acc_exp += exp
                table_rows.append((seq, req, acc_req, exp, acc_exp))
            reached_seq = current_round
        else:
            reached_seq = 0
            for item in grouped_rows.get(pgid, []):
                seq = int(item["seq"])
                req = int(item["requirement"])
                exp = int(item.get("exp", 0))
                acc_exp += exp
                if current >= req:
                    reached_seq = seq
                table_rows.append((seq, req, req, exp, acc_exp))
        sections.append((row, table_rows, reached_seq))

    chara = await ctx.md.game_characters.find_by_id(cid)
    chara_name = f"{chara.get('firstName', '')}{chara.get('givenName', '')}" if chara else (get_character_first_nickname(cid) or str(cid))

    style_h = TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=BLACK)
    style_t = TextStyle(font=DEFAULT_FONT, size=18, color=(55, 55, 55))
    style_cur = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(200, 60, 50))
    widths = (78, 105, 125, 78, 112)

    def draw_table(row: dict, table_rows: list[tuple[int, int, int, int, int]], reached_seq: int):
        chunk_size = 40
        chunks = [table_rows[i:i + chunk_size] for i in range(0, len(table_rows), chunk_size)] or [[]]
        with VSplit().set_bg(roundrect_bg(fill=(255, 255, 255, 140))).set_padding(12).set_sep(10).set_content_align('lt').set_item_align('lt'):
            TextBox(
                f"{row['title']} 当前进度 {row['current']:,}",
                TextStyle(font=DEFAULT_BOLD_FONT, size=22, color=BLACK),
            )
            _cr_test_progress_bar(row["current"], row["upper"], row["ratio"], 560)
            with HSplit().set_content_align('lt').set_item_align('lt').set_sep(12):
                for chunk in chunks:
                    with VSplit().set_content_align('c').set_item_align('c').set_sep(4):
                        with HSplit().set_content_align('c').set_item_align('c').set_sep(4):
                            for title, width in zip(("档位", "需求", "累计需求", "EXP", "累计EXP"), widths):
                                TextBox(title, style_h).set_size((width, 36)).set_content_align('c')
                        for seq, req, acc_req, exp, acc_exp in chunk:
                            bg = FillBg((255, 244, 196, 210)) if seq == reached_seq and reached_seq > 0 else roundrect_bg(fill=(255, 255, 255, 150))
                            with HSplit().set_content_align('c').set_item_align('c').set_sep(4).set_bg(bg):
                                values = (f"#{seq}", str(req), str(acc_req), str(exp), str(acc_exp))
                                for value, width in zip(values, widths):
                                    TextBox(value, style_cur if seq == reached_seq else style_t).set_size((width, 34)).set_content_align('c')

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12):
            await get_detailed_profile_card(ctx, profile, err_msg)
            with HSplit().set_content_align('l').set_item_align('c').set_sep(8).set_padding(12).set_bg(roundrect_bg()):
                ImageBox(get_chara_icon_by_chara_id(cid), size=(48, 48))
                title = CR_TEST_MISSION_SHORT_NAMES.get(mission_type, mission_type)
                TextBox(f"[CR测试] {chara_name} {title} 任务详览", TextStyle(font=DEFAULT_BOLD_FONT, size=26, color=BLACK))
            for row, table_rows, reached_seq in sections:
                draw_table(row, table_rows, reached_seq)

    add_watermark(canvas)
    return await canvas.get_img()


cr_test_character_rank_mission = SekaiCmdHandler([
    "/cr任务",
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
该命令为CR新实现测试入口，不占用 /角色等级任务。
""".strip()

    raw_args = ctx.get_args().strip()
    assert_and_reply(raw_args, help_msg)
    if raw_args.lower() in ("help", "帮助"):
        return await ctx.asend_reply_msg(help_msg)

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
    img = await compose_cr_test_character_rank_mission_overview_image(ctx, profile, err_msg, cid)
    return await ctx.asend_reply_msg(await get_image_cq(img, low_quality=True))

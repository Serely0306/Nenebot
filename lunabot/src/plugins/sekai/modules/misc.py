import asyncio
from ...utils import *
from ...record import after_record_hook
from ..common import *
from ..handler import *
from ..asset import *
from ..draw import *
from ..sub import SekaiGroupSubHelper, SekaiUserSubHelper
from .card_extractor import CardExtractor, CardExtractResult, CardThumbnail
from ..gameapi import get_gameapi_config, request_gameapi
from .profile import (
    get_card_full_thumbnail, 
    get_basic_profile,
    build_profile_play_section,
    get_detailed_profile,
    get_detailed_profile_card,
    get_detailed_profile_card_filter,
    get_player_bind_id,
    get_player_bind_count,
    process_hide_uid,
)
from .card import (
    has_after_training, 
    only_has_after_training,
    get_character_name_by_id,
    get_card_full_thumbnail,
    get_card_image,
    get_character_sd_image,
)
from .education import (
    build_challenge_live_detail_section,
    build_power_bonus_detail_section,
)
from .event import get_card_supply_type
from .resbox import get_res_icon


md_update_group_sub = SekaiGroupSubHelper("update", "MasterData更新通知", ALL_SERVER_REGIONS)
ad_result_sub = SekaiUserSubHelper("ad", "广告奖励推送", ['jp'], hide=True)


# ======================= 指令处理 ======================= #

pjsk_update = SekaiCmdHandler([
    "/pjsk update", "/pjsk refresh", "/pjsk更新",
])
pjsk_update.check_cdrate(cd).check_wblist(gbl)
@pjsk_update.handle()
async def _(ctx: SekaiHandlerContext):
    mgr = RegionMasterDbManager.get(ctx.region)
    msg = f"{get_region_name(ctx.region)}MasterData数据源"
    for source in await mgr.get_all_sources(force_update=True):
        msg += f"\n[{source.name}] {source.version}"
    return await ctx.asend_reply_msg(msg.strip())

pjsk_update1 = SekaiCmdHandler([
    "/pjsk update masterdb",
])
pjsk_update1.check_cdrate(cd).check_wblist(gbl)

@pjsk_update1.handle()
async def _(ctx: SekaiHandlerContext):
    region = ctx.region
    db_mgr = RegionMasterDbManager.get(region)

    # 1) 刷新版本信息（你原来的行为）
    await db_mgr.update()
    source = await db_mgr.get_latest_source()

    # 2) 收集“本地缓存过的 masterdata 名称”
    versions = file_db.get_copy("master_data_cache_versions", {}).get(region, {})
    names = sorted(versions.keys())

    if not names:
        msg = (
            f"{get_region_name(region)}MasterData数据源"
            f"\n最新版本: [{source.name}] {source.version}"
            f"\n本地没有已缓存的 MasterData 文件（先调用一次相关功能生成缓存，或你自己提供全量文件名列表）"
        )
        return await ctx.asend_reply_msg(msg)

    # 3) 并发强制更新（限流避免把源站打爆/自己超时）
    sem = asyncio.Semaphore(6)  # 并发数自己调

    ok, fail = [], []

    async def one(name: str):
        async with sem:
            try:
                await MasterDataManager.get(name).force_update(region, source)
                ok.append(name)
            except Exception as e:
                fail.append((name, str(e)))

    await asyncio.gather(*[one(n) for n in names])

    # 4) 输出结果
    msg = (
        f"{get_region_name(region)}MasterData 强制更新完成"
        f"\n使用数据源: [{source.name}] {source.version}"
        f"\n成功: {len(ok)} 失败: {len(fail)}"
    )
    if fail:
        # 避免刷屏，截断一下
        lines = "\n".join([f"- {n}: {err[:60]}" for n, err in fail[:10]])
        msg += f"\n失败示例(前10):\n{lines}"

    return await ctx.asend_reply_msg(msg.strip())

ngword = SekaiCmdHandler([
    "/pjsk ng", "/pjsk ngword", "/pjsk ng word",
    "/pjsk屏蔽词", "/pjsk屏蔽", "/pjsk敏感", "/pjsk敏感词",
])
ngword.check_cdrate(cd).check_wblist(gbl)
@ngword.handle()
async def _(ctx: SekaiHandlerContext):
    text = ctx.get_args()
    assert_and_reply(text, "请输入要查询的文本")
    words = await ctx.md.ng_words.get()
    def check():
        ret = []
        for word in words:
            if word in text:
                ret.append(word)
        return ret
    ret = await run_in_pool(check)
    if ret:
        await ctx.asend_reply_msg(f"检测到屏蔽词：{', '.join(ret)}")
    else:
        await ctx.asend_reply_msg("未检测到屏蔽词")


upload_help = SekaiCmdHandler([
    "/抓包帮助", "/抓包", "/pjsk upload help",
])
upload_help.check_cdrate(cd).check_wblist(gbl)
@upload_help.handle()
async def _(ctx: SekaiHandlerContext):
    text = Path(f"{SEKAI_CONFIG_DIR}/upload_help.txt").read_text(encoding="utf-8")
    return await ctx.asend_fold_msg_adaptive(text.strip(), threshold=3, need_reply=False)


card_extractor = CardExtractor()
extract_card = SekaiCmdHandler([
    "/提取卡牌"
], regions=['jp'])
extract_card.check_cdrate(cd).check_wblist(gbl)
@extract_card.handle()
async def _(ctx: SekaiHandlerContext):
    await ctx.block()
    global card_extractor
    bot, event = ctx.bot, ctx.event
    reply_msg = ctx.get_reply_msg()
    assert_and_reply(reply_msg, f"请回复一张图片")
    cqs = extract_cq_code(reply_msg)
    assert_and_reply('image' in cqs, f"请回复一张图片")
    img = await download_image(cqs['image'][0]['url'])
    
    if not card_extractor.is_initialized():
        card_thumbs = []
        for card in await ctx.md.cards.get():
            card_id = card['id']
            rarity = card['cardRarityType']
            attr = card['attr']
            assetbundle_name = card['assetbundleName']
            img_dir = 'data/sekai/assets/rip/jp/thumbnail/chara_rip'
            if not only_has_after_training(card):
                normal_path = await ctx.rip.get_asset_cache_path(f'thumbnail/chara_rip/{assetbundle_name}_normal.png')
                if normal_path:
                    card_thumbs.append(CardThumbnail(
                        id=card_id,
                        rarity=rarity,
                        attr=attr,
                        is_aftertraining=False,
                        img_path=os.path.join(img_dir, f"{assetbundle_name}_normal.png"),
                    ))
            if has_after_training(card):
                aftertraining_path = await ctx.rip.get_asset_cache_path(f'thumbnail/chara_rip/{assetbundle_name}_after_training.png')
                if aftertraining_path:
                    card_thumbs.append(CardThumbnail(
                        id=card_id,
                        rarity=rarity,
                        attr=attr,
                        is_aftertraining=True,
                        img_path=os.path.join(img_dir, f"{assetbundle_name}_after_training.png"),
                    ))
        t = datetime.now()
        await run_in_pool(card_extractor.init, card_thumbs)
        logger.info(f"CardExtractor initialized in {datetime.now() - t} seconds")
    
    t = datetime.now()
    result: CardExtractResult = await run_in_pool(card_extractor.extract_cards, img)
    logger.info(f"CardExtractor extracted {len(result.cards)} cards in {datetime.now() - t} seconds")
    
    with Canvas(bg=FillBg(WHITE)).set_padding(BG_PADDING) as canvas:
        with Grid(col_count=result.grid.cols).set_sep(8, 8):
            for row_idx in range(result.grid.rows):
                for col_idx in range(result.grid.cols):
                    with HSplit().set_sep(0):
                        w = 64
                        try:
                            import cv2
                            img = result.grid.get_grid_image(row_idx, col_idx)
                            img = cv2.cvtColor(img, cv2.COLOR_BGR2RGB)
                            img = Image.fromarray(img)
                            ImageBox(img, size=(w, w))
                        except:
                            Spacer(w, w)
                            Spacer(w, w)
                            continue

                        card = find_by_predicate(result.cards, lambda c: c.row_idx == row_idx and c.col_idx == col_idx)
                        if card is None:
                            ImageBox(UNKNOWN_IMG, size=(w, w))
                        else:
                            pcard = {
                                'defaultImage': "special_training" if card.is_aftertraining else "normal",
                                'specialTrainingStatus': "done" if card.is_aftertraining else "none",
                                'level': card.level,
                                'masterRank': card.master_rank,
                            }
                            custom_text = None if card.level is not None else f"SLv.{card.skill_level}"
                            thumb = await get_card_full_thumbnail(ctx, card.id, pcard=pcard, custom_text=custom_text)
                            ImageBox(thumb, size=(w, w))
        
    return await ctx.asend_reply_msg(
        await get_image_cq(
            await canvas.get_img(),
            low_quality=True,
        )
    )


chara_bd = SekaiCmdHandler([
    "/pjsk chara birthday", "/角色生日", "/生日",
])
chara_bd.check_cdrate(cd).check_wblist(gbl)
@chara_bd.handle()
async def _(ctx: SekaiHandlerContext):
    # 获取角色生日信息
    async def get_bd_info(cid: int) -> dict:
        info = { 'cid': cid }
        m, d = get_character_birthday(cid)
        info['month'] = m
        info['day'] = d
        info['next'] = get_character_next_birthday_dt(ctx.region, cid)

        for card in await ctx.md.cards.get():
            if card['characterId'] == cid and card['cardRarityType'] == 'rarity_birthday':
                info.setdefault('cards', []).append(card)
        return info

    args = ctx.get_args().strip()

    bd_infos: list[dict] = [await get_bd_info(i) for i in range(1, 27)]
    bd_infos.sort(key=lambda x: x['next'])

    # 判断是否五周年
    is_fifth_anniv = is_fifth_anniversary(ctx.region)

    if not args:
        info = bd_infos[0]
    elif args.isdigit():
        idx = int(args) - 1
        assert_and_reply(0 <= idx < len(bd_infos), "角色生日索引超出范围")
        info = bd_infos[idx]
    else:
        cid = get_cid_by_nickname(args)
        assert_and_reply(cid, f"""
使用方式:
查询最近的角色生日: "{ctx.original_trigger_cmd}"
查询第二近的角色生日: "{ctx.original_trigger_cmd} 2"
查询指定角色下次生日: "{ctx.original_trigger_cmd} 角色名"
""".strip())
        info = find_by(bd_infos, 'cid', cid)

    style1 = TextStyle(DEFAULT_BOLD_FONT, 24, BLACK)
    style2 = TextStyle(DEFAULT_FONT, 20, BLACK)

    card_thumbs = await batch_gather(*[get_card_full_thumbnail(ctx, card, False) for card in info['cards']])
    card_image = await get_card_image(ctx, random.choice(info['cards']), False)
    next_time: datetime = info['next']
    month = info['month']
    day = info['day']

    if is_fifth_anniv:
        gacha_start,    gacha_end   = next_time - timedelta(days=4), next_time + timedelta(days=3)
        live_start,     live_end    = next_time - timedelta(days=0), next_time + timedelta(days=1)
        drop_start,     drop_end    = next_time - timedelta(days=3), next_time + timedelta(days=0)
        flower_start,   flower_end  = next_time - timedelta(days=3), next_time + timedelta(days=3)
        party_start,    party_end   = next_time - timedelta(days=0), next_time + timedelta(days=3)
    else:
        gacha_start,    gacha_end   = next_time - timedelta(days=0), next_time + timedelta(days=7)
        live_start,     live_end    = next_time - timedelta(days=0), next_time + timedelta(days=1)

    def draw_time_range(label: str, start: datetime, end: datetime):
        end = end - timedelta(minutes=1)
        with HSplit().set_sep(8).set_content_align('l').set_item_align('l'):
            TextBox(f"{label} ", style1)
            start_text = f"{start.strftime('%m-%d %H:%M')}({get_readable_datetime(start, False)})"
            end_text = f"{end.strftime('%m-%d %H:%M')}({get_readable_datetime(end, False)})"
            TextBox(f"{start_text} ~ {end_text}", style2)

    cid = info['cid']
    colorcode = (await ctx.md.game_character_units.find_by_id(cid))['colorCode']

    with Canvas(bg=ImageBg(card_image)).set_padding(BG_PADDING) as canvas:
        with VSplit().set_content_align('c').set_item_align('c').set_padding(16).set_sep(8) \
            .set_item_bg(roundrect_bg()).set_bg(roundrect_bg()):
        
            with HSplit().set_sep(16).set_padding(16).set_content_align('c').set_item_align('c'):
                ImageBox(await get_character_sd_image(cid), size=(None, 80), shadow=True)
                title_img = await SekaiHandlerContext.from_region("jp").rip.img(f"character/label_horizontal/chr_h_lb_{cid}.png")
                ImageBox(title_img, size=(None, 60))
                TextBox(f"{month}月{day}日", 
                        TextStyle(DEFAULT_HEAVY_FONT, 32, (100, 100, 100), 
                                  use_shadow=True, shadow_offset=2, shadow_color=color_code_to_rgb(colorcode)))

            with VSplit().set_sep(4).set_padding(16).set_content_align('l').set_item_align('l'):
                with HSplit().set_sep(8).set_padding(0).set_content_align('l').set_item_align('l'):
                    TextBox(f"({get_region_name(ctx.region)}) 距离下次生日还有{(next_time - datetime.now()).days}天", style1)
                    Spacer(w=16)
                    TextBox(f"应援色", style1)
                    TextBox(colorcode, TextStyle(DEFAULT_FONT, 20, ADAPTIVE_WB)) \
                        .set_bg(RoundRectBg(color_code_to_rgb(colorcode), radius=4)).set_padding(8)

                draw_time_range("🎰卡池开放时间", gacha_start, gacha_end)
                draw_time_range("🎤虚拟LIVE时间", live_start, live_end)

            if is_fifth_anniv:
                with VSplit().set_sep(4).set_padding(16).set_content_align('l').set_item_align('l'):
                    draw_time_range("💧露滴掉落时间", drop_start, drop_end)
                    draw_time_range("🌱浇水开放时间", flower_start, flower_end)
                    draw_time_range("🎂派对开放时间", party_start, party_end)

            with HSplit().set_sep(4).set_padding(16).set_content_align('l').set_item_align('l'):
                TextBox(f"卡牌", style1)
                Spacer(w=8)
                with Grid(col_count=6).set_sep(4, 4):
                    for i in range(len(card_thumbs)):
                        with VSplit().set_sep(2).set_content_align('c').set_item_align('c'):
                            ImageBox(card_thumbs[i], size=(80, 80), shadow=True)
                            TextBox(f"{info['cards'][i]['id']}", TextStyle(DEFAULT_FONT, 16, (50, 50, 50)))
                
            with Grid(col_count=13).set_sep(2, 2).set_padding(16).set_content_align('c').set_item_align('c'):
                idx = 0
                start_cid = 6
                for i, item in enumerate(bd_infos):
                    if item['cid'] == start_cid:
                        idx = i
                        break
                for _ in range(len(bd_infos)):
                    chara_id = bd_infos[idx % len(bd_infos)]['cid']
                    idx += 1
                    with VSplit().set_sep(0).set_content_align('c').set_item_align('c'):
                        b = ImageBox(get_chara_icon_by_chara_id(chara_id), size=(40, 40)).set_padding(4)
                        if chara_id == cid:
                            b.set_bg(roundrect_bg(radius=8))
                        month, day = get_character_birthday(chara_id)
                        TextBox(f"{month}/{day}", TextStyle(DEFAULT_FONT, 14, (50, 50, 80)))

    add_watermark(canvas)

    return await ctx.asend_reply_msg(
        await get_image_cq(
            await canvas.get_img(),
            low_quality=True,
        )
    )
            


heyiwei = SekaiCmdHandler(["/pjsk detail", ])
heyiwei.check_cdrate(cd).check_wblist(gbl)

def get_pjsk_detail_missing_lines(profile: dict) -> list[str]:
    # 逐字段提示 suite 缺失内容，便于直接判断是远端不提供还是数据本身缺失。
    required_keys = [
        'userGamedata', 'userChargedCurrency', 'userBoostItems', 'userMaterials',
        'userCards',
        'userAreas', 'userCharacters', 'userMysekaiGates', 'userMysekaiFixtureGameCharacterPerformanceBonuses',
        'userChallengeLiveSoloResults', 'userChallengeLiveSoloStages', 'userChallengeLiveSoloHighScoreRewards',
        'userCharacterMissionV2s', 'userCharacterMissionV2Statuses',
    ]
    return [f"你的Suite数据源没有提供{key}数据" for key in required_keys if key not in profile]

async def get_pjsk_detail_card_stats(ctx: SekaiHandlerContext, profile: dict) -> dict:
    user_cards = profile.get('userCards', [])
    card_ids = [item['cardId'] for item in user_cards]
    cards = await ctx.md.cards.collect_by_ids(card_ids)
    cards_dict = {card['id']: card for card in cards}

    stats = {
        'total': len(user_cards),
        'rarity_4': 0,
        'rarity_3': 0,
        'birthday': 0,
        'limited': 0,
        'fes': 0,
        'msr5': 0,
    }

    limited_types = {'term_limited', 'unit_event_limited', 'collaboration_limited'}
    fes_types = {'colorful_festival_limited', 'bloom_festival_limited'}
    for user_card in user_cards:
        card = cards_dict.get(user_card['cardId'])
        if not card:
            continue
        rarity = card['cardRarityType']
        if rarity == 'rarity_4':
            stats['rarity_4'] += 1
        elif rarity == 'rarity_3':
            stats['rarity_3'] += 1
        elif rarity == 'rarity_birthday':
            stats['birthday'] += 1

        if rarity in ('rarity_4', 'rarity_birthday') and user_card.get('masterRank', 0) >= 5:
            stats['msr5'] += 1

        supply_type = await get_card_supply_type(ctx, user_card['cardId'])
        if supply_type in limited_types:
            stats['limited'] += 1
        elif supply_type in fes_types:
            stats['fes'] += 1
    return stats

async def get_pjsk_detail_total_boost_energy(ctx: SekaiHandlerContext, profile: dict) -> int:
    boost_items = {item['id']: item for item in await ctx.md.boost_items.get()}
    total = 0
    for item in profile.get('userBoostItems', []):
        boost_item = boost_items.get(item['boostItemId'])
        if not boost_item:
            continue
        total += boost_item.get('recoveryValue', 0) * item.get('quantity', 0)
    return total

async def get_pjsk_detail_material_items(ctx: SekaiHandlerContext, profile: dict) -> list[dict]:
    user_materials = {item['materialId']: item.get('quantity', 0) for item in profile.get('userMaterials', [])}
    materials = await ctx.md.materials.get()
    special_ids = {11, 12, 13, 14, 47, 48}
    result = []
    for material in materials:
        mid = material['id']
        quantity = user_materials.get(mid, 0)
        in_common_group = material.get('materialType') in {'common', 'master_lesson'}
        in_special_group = mid in special_ids
        if not in_common_group and not in_special_group:
            continue
        if mid > 50 and quantity <= 0:
            continue
        result.append({
            'id': mid,
            'name': material.get('name', f"ID:{mid}"),
            'quantity': quantity,
            'icon': await get_res_icon(ctx, 'material', mid),
        })
    result.sort(key=lambda x: x['id'])
    return result

def build_pjsk_detail_basic_profile(profile: dict) -> dict:
    # 基础 profile 接口不可用时，用 suite 中已有字段回填最小资料结构。
    user_gamedata = profile.get('userGamedata', {})
    deck_id = user_gamedata.get('deck')
    return {
        'user': {
            'userId': user_gamedata.get('userId', 0),
            'name': user_gamedata.get('name', '?'),
            'rank': user_gamedata.get('rank', 0),
        },
        'userDeck': find_by(profile.get('userDecks', []), 'deckId', deck_id) or {},
        'userCards': profile.get('userCards', []),
        'userProfile': profile.get('userProfile', {}),
        'userProfileHonors': profile.get('userProfileHonors', []),
        # 打歌统计仍按原本 profile 接口逻辑获取，这里不从 suite 回填。
        'userMusicDifficultyClearCount': [],
    }

async def build_pjsk_detail_value_panel(
    ctx: SekaiHandlerContext,
    title: str,
    items: list[tuple[str, int, str | None]],
    section_title_style: TextStyle,
    item_title_style: TextStyle,
    item_value_style: TextStyle,
    *,
    col_count: int = 4,
    item_size: tuple[int, int] = (136, 82),
) -> Frame:
    # 统一资源统计小卡片布局，便于和卡面区对齐。
    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_padding(16).set_bg(roundrect_bg()) as ret:
        TextBox(title, section_title_style)
        with Grid(col_count=col_count).set_content_align('l').set_item_align('l').set_sep(10, 10):
            for item_title, item_value, res_type in items:
                with VSplit().set_content_align('l').set_item_align('l').set_sep(5).set_padding(10).set_size(item_size).set_bg(roundrect_bg(fill=(255, 255, 255, 180))):
                    with HSplit().set_content_align('l').set_item_align('c').set_sep(8):
                        if res_type:
                            icon_id = 1 if res_type == 'boost_item' else None
                            ImageBox(await get_res_icon(ctx, res_type, icon_id), size=(None, 24))
                        TextBox(item_title, item_title_style.replace(size=16)).set_overflow('clip')
                    TextBox(str(item_value), item_value_style.replace(size=20))
    return ret

async def build_pjsk_detail_material_panel(
    title: str,
    material_items: list[dict],
    section_title_style: TextStyle,
    text_style: TextStyle,
    *,
    col_count: int = 4,
    item_size: tuple[int, int] = (252, 80),
) -> Frame:
    # 材料区固定成多列网格，控制整体高度并保留原始数量文本。
    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_padding(16).set_bg(roundrect_bg()) as ret:
        TextBox(title, section_title_style)
        with Grid(col_count=col_count).set_content_align('l').set_item_align('l').set_sep(12, 12):
            for item in material_items:
                with HSplit().set_content_align('l').set_item_align('c').set_sep(8).set_padding(12).set_size(item_size).set_bg(roundrect_bg(fill=(255, 255, 255, 180))):
                    ImageBox(item['icon'], size=(32, 32))
                    with VSplit().set_content_align('l').set_item_align('l').set_sep(2):
                        TextBox(item['name'], text_style).set_w(item_size[0] - 76).set_overflow('clip')
                        TextBox(f"x{item['quantity']}", TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(50, 50, 50)))
    return ret

async def build_pjsk_detail_deck_panel(
    ctx: SekaiHandlerContext,
    profile: dict,
    card_stat_items: list[tuple[str, int, str | None]],
    section_title_style: TextStyle,
    item_title_style: TextStyle,
    item_value_style: TextStyle,
    *,
    col_count: int = 3,
    stat_item_size: tuple[int, int] = (146, 76),
    thumb_size: int = 84,
) -> Frame:
    # 当前编组和卡牌汇总放在同一块，方便和右侧资源区做等高布局。
    deck_id = profile['userGamedata']['deck']
    deck = find_by(profile.get('userDecks', []), 'deckId', deck_id) or {}
    pcards = [find_by(profile.get('userCards', []), 'cardId', deck.get(f'member{i}')) for i in range(1, 6)]
    pcards = [pcard for pcard in pcards if pcard]
    for pcard in pcards:
        pcard['after_training'] = pcard['defaultImage'] == "special_training" and pcard['specialTrainingStatus'] == "done"

    card_ids = [pcard['cardId'] for pcard in pcards]
    cards = await ctx.md.cards.collect_by_ids(card_ids)
    card_map = {card['id']: card for card in cards}
    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_padding(16).set_bg(roundrect_bg()) as ret:
        TextBox("卡面", section_title_style)
        with HSplit().set_content_align('c').set_item_align('c').set_sep(8):
            for pcard in pcards:
                card = card_map.get(pcard['cardId'])
                if not card:
                    continue
                thumb = await get_card_full_thumbnail(ctx, card, pcard=pcard)
                ImageBox(thumb, size=(thumb_size, thumb_size), image_size_mode='fill', shadow=True)
        with Grid(col_count=col_count).set_content_align('l').set_item_align('l').set_sep(10, 10):
            for item_title, item_value, _ in card_stat_items:
                with VSplit().set_content_align('l').set_item_align('l').set_sep(4).set_padding(10).set_size(stat_item_size).set_bg(roundrect_bg(fill=(255, 255, 255, 180))):
                    TextBox(item_title, item_title_style.replace(size=16)).set_overflow('clip')
                    TextBox(str(item_value), item_value_style.replace(size=20))
    return ret

async def build_pjsk_detail_leader_panel(
    ctx: SekaiHandlerContext,
    profile: dict,
    section_title_style: TextStyle,
) -> Frame:
    # 保持队长次数的原口径，只抽成 section 给 pjsk detail 复用。
    ucms = profile.get('userCharacterMissionV2s')
    ucm_ss = profile.get('userCharacterMissionV2Statuses')
    assert_and_reply(ucms, "你的Suite数据源没有提供userCharacterMissionV2s数据")
    assert_and_reply(ucm_ss, "你的Suite数据源没有提供userCharacterMissionV2Statuses数据")

    max_playcount = 0
    ex_seq_pc_list: list[tuple[int, int]] = []
    for item in await ctx.md.character_mission_v2_parameter_groups.find_by('id', 1, mode='all'):
        max_playcount = max(max_playcount, item['requirement'])
    for item in await ctx.md.character_mission_v2_parameter_groups.find_by('id', 101, mode='all'):
        ex_seq_pc_list.append((item['seq'], item['requirement']))
    ex_seq_pc_list.append((100000, 0))
    ex_seq_pc_list.sort()

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
        for i in range(len(ex_seq_pc_list) - 1):
            if ex_seq_pc_list[i + 1][0] > seq:
                playcounts_ex[cid] += ex_seq_pc_list[i][1]
                break

    header_style = TextStyle(font=DEFAULT_BOLD_FONT, size=18, color=(25, 25, 25, 255))
    text_style = TextStyle(font=DEFAULT_FONT, size=16, color=(50, 50, 50, 255))
    w1, w2, w3, w4, w5 = 64, 88, 76, 88, 240
    with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12) as ret:
        TextBox("队长次数", section_title_style).set_padding((8, 0))
        with VSplit().set_content_align('l').set_item_align('l').set_sep(8).set_padding(16).set_bg(roundrect_bg()):
            with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(46).set_padding(4).set_bg(roundrect_bg()):
                TextBox("角色", header_style).set_w(w1).set_content_align('c')
                TextBox("队长", header_style).set_w(w2).set_content_align('c')
                TextBox("EX级别", header_style).set_w(w3).set_content_align('c')
                TextBox("EX次数", header_style).set_w(w4).set_content_align('c')
                TextBox(f"进度({max_playcount})", header_style).set_w(w5).set_content_align('c')

            for idx, cid in enumerate(range(1, 27)):
                bg_color = (255, 255, 255, 150) if idx % 2 == 0 else (255, 255, 255, 100)
                pc = playcounts.get(cid, 0)
                pc_text = "-" if cid not in playcounts else str(pc)
                pc_ex_text = "-" if cid not in playcounts_ex else str(playcounts_ex[cid])
                ex_level_text = "-" if cid not in ex_level else f"x{ex_level[cid]}"

                with HSplit().set_content_align('c').set_item_align('c').set_sep(8).set_h(38).set_padding(4).set_bg(roundrect_bg(fill=bg_color)):
                    with Frame().set_w(w1).set_content_align('c'):
                        ImageBox(get_chara_icon_by_chara_id(cid), size=(None, 30))
                    TextBox(pc_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w2).set_content_align('c')
                    TextBox(ex_level_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w3).set_content_align('c')
                    TextBox(pc_ex_text, text_style.replace(font=DEFAULT_BOLD_FONT)).set_w(w4).set_content_align('c')
                    with Frame().set_w(w5).set_content_align('lt'):
                        progress = max(min(pc / max_playcount, 1), 0) if max_playcount else 0
                        total_w, total_h, border = w5, 12, 2
                        progress_w = int((total_w - border * 2) * progress)
                        color = (255, 50, 50, 255)
                        if pc > 50000:
                            color = (100, 255, 100, 255)
                        elif pc > 40000:
                            color = (255, 255, 100, 255)
                        elif pc > 30000:
                            color = (255, 200, 100, 255)
                        elif pc > 20000:
                            color = (255, 150, 100, 255)
                        elif pc > 10000:
                            color = (255, 100, 100, 255)
                        Spacer(w=total_w, h=total_h).set_bg(RoundRectBg(fill=(100, 100, 100, 180), radius=total_h // 2))
                        if progress_w > 0:
                            Spacer(w=progress_w, h=total_h - border * 2).set_bg(RoundRectBg(fill=color, radius=(total_h - border) // 2)).set_offset((border, border))
    return ret


async def render_frame_to_image(frame: Frame, scale: float | None = None) -> Image.Image:
    # 右栏 section 先在 Sekai 蓝底上单独出图，再按目标宽度等比缩放，
    # 这样 roundrect 的半透明背景会和主图保持一致，不会发白。
    with Canvas(bg=SEKAI_BLUE_BG) as canvas:
        pass
    canvas.add_item(frame)
    return await canvas.get_img(scale=scale)

async def compose_pjsk_detail_image(ctx: SekaiHandlerContext, qid: int) -> Image.Image:
    # 综合拼接资料、资源、材料、加成、挑战和队长次数到一张详情图。
    profile, err_msg = await get_detailed_profile(
        ctx,
        qid,
        filter=get_detailed_profile_card_filter(
            'userGamedata',
            'userChargedCurrency',
            'userMaterials',
            'userBoostItems',
            'userCards',
            'userAreas',
            'userCharacters',
            'userMysekaiGates',
            'userMysekaiFixtureGameCharacterPerformanceBonuses',
            'userChallengeLiveSoloResults',
            'userChallengeLiveSoloStages',
            'userChallengeLiveSoloHighScoreRewards',
            'userCharacterMissionV2s',
            'userCharacterMissionV2Statuses',
            'userProfile',
            'userProfileHonors',
        ),
        raise_exc=True,
    )
    try:
        basic_profile = await get_basic_profile(ctx, profile['userGamedata']['userId'], raise_when_no_found=False)
    except Exception:
        basic_profile = {}
    if not basic_profile:
        basic_profile = build_pjsk_detail_basic_profile(profile)

    missing_lines = get_pjsk_detail_missing_lines(profile)
    total_boost_energy = await get_pjsk_detail_total_boost_energy(ctx, profile)
    card_stats = await get_pjsk_detail_card_stats(ctx, profile)
    material_items = await get_pjsk_detail_material_items(ctx, profile)

    user_gamedata = profile.get('userGamedata', {})
    charged = profile.get('userChargedCurrency', {})
    summary_items = [
        ("金币", user_gamedata.get('coin', 0), 'coin'),
        ("等级", user_gamedata.get('rank', 0), None),
        ("虚拟币", user_gamedata.get('virtualCoin', 0), 'virtual_coin'),
        ("免费水晶", charged.get('free', 0), 'jewel'),
        ("付费水晶", charged.get('paid', 0), 'paid_jewel'),
        ("总水晶", charged.get('free', 0) + charged.get('paid', 0), 'jewel'),
        ("演出能量", total_boost_energy, 'boost_item'),
    ]
    card_stat_items = [
        ("总卡数", card_stats['total'], None),
        ("四星", card_stats['rarity_4'], None),
        ("三星", card_stats['rarity_3'], None),
        ("生日", card_stats['birthday'], None),
        ("限定", card_stats['limited'], None),
        ('Fes/BFes', card_stats['fes'], None),
        ('MSR5', card_stats['msr5'], None),
    ]

    section_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=26, color=(20, 20, 20))
    item_title_style = TextStyle(font=DEFAULT_BOLD_FONT, size=20, color=(40, 40, 40))
    item_value_style = TextStyle(font=DEFAULT_BOLD_FONT, size=24, color=(30, 30, 30))
    text_style = TextStyle(font=DEFAULT_FONT, size=18, color=(60, 60, 60))
    red_style = TextStyle(font=DEFAULT_FONT, size=18, color=(180, 20, 20))
    # 左上两列重新分配宽度，给简易信息更多空间，同时保持下方总宽度不变。
    left_col_1_w = 560
    left_col_2_w = 620
    left_total_w = left_col_1_w + left_col_2_w + 16
    top_row_h = 210
    second_row_h = 340
    leader_panel = await build_pjsk_detail_leader_panel(ctx, profile, section_title_style)
    leader_img = await render_frame_to_image(leader_panel)
    right_col_w = leader_img.width
    challenge_section = await build_challenge_live_detail_section(ctx, profile)
    challenge_img = await render_frame_to_image(
        challenge_section,
        scale=right_col_w / max(challenge_section._get_self_size()[0], 1),
    )

    with Canvas(bg=SEKAI_BLUE_BG).set_padding(BG_PADDING) as canvas:
        with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                    (await get_detailed_profile_card(ctx, profile, err_msg)).set_w(left_col_1_w).set_h(top_row_h)
                    (await build_profile_play_section(ctx, basic_profile, profile, compact=True)).set_w(left_col_2_w).set_h(top_row_h)

                with HSplit().set_content_align('lt').set_item_align('lt').set_sep(16):
                    (await build_pjsk_detail_deck_panel(
                        ctx,
                        profile,
                        card_stat_items,
                        section_title_style,
                        item_title_style,
                        item_value_style,
                        col_count=4,
                        stat_item_size=(108, 70),
                        thumb_size=78,
                    )).set_w(left_col_1_w).set_h(second_row_h)
                    (await build_pjsk_detail_value_panel(
                        ctx,
                        "资源",
                        summary_items,
                        section_title_style,
                        item_title_style,
                        item_value_style,
                        col_count=4,
                        item_size=(136, 82),
                    )).set_w(left_col_2_w).set_h(second_row_h)

                (await build_pjsk_detail_material_panel(
                    "材料",
                    material_items,
                    section_title_style,
                    text_style,
                    col_count=4,
                    item_size=(252, 80),
                )).set_w(left_total_w)

                with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_w(left_total_w):
                    TextBox("加成信息", section_title_style).set_padding((8, 0))
                    (await build_power_bonus_detail_section(ctx, profile)).set_w(left_total_w)
                if missing_lines:
                    with VSplit().set_content_align('l').set_item_align('l').set_sep(4).set_padding((12, 8)).set_w(left_total_w).set_bg(roundrect_bg(fill=(255, 240, 240, 220))):
                        for line in missing_lines:
                            TextBox(line, red_style, use_real_line_count=True)

            with VSplit().set_content_align('lt').set_item_align('lt').set_sep(12).set_w(right_col_w):
                TextBox("挑战信息", section_title_style).set_padding((8, 0))
                ImageBox(challenge_img, size=(challenge_img.width, challenge_img.height))
                ImageBox(leader_img, size=(leader_img.width, leader_img.height))

    add_watermark(canvas)
    return await canvas.get_img()

@heyiwei.handle()
async def _(ctx: SekaiHandlerContext):
    return await ctx.asend_reply_msg(await get_image_cq(
        await compose_pjsk_detail_image(ctx, ctx.user_id),
        low_quality=True,
    ))


# ======================= 定时通知 ======================= #

# masterdata更新通知
@RegionMasterDbManager.on_update()
async def send_masterdata_update_notify(
    region: str, source: str,
    version: str, last_version: str,
    asset_version: str, last_asset_version: str,
):
    region_name = get_region_name(region)

    # 防止重复通知
    last_notified_version = file_db.get(f"last_notified_md_version_{region}", None)
    if last_notified_version and get_version_order(last_notified_version) >= get_version_order(version):
        return
    file_db.set(f"last_notified_md_version_{region}", version)

    msg = f"从{source}获取{region_name}的MasterData版本更新: {last_version} -> {version}\n"
    if last_asset_version != asset_version:
        msg += f"解包资源版本: {last_asset_version} -> {asset_version}\n"
    msg = msg.strip()

    for group_id in md_update_group_sub.get_all(region):
        if not gbl.check_id(group_id): continue
        try:
            await send_group_msg_by_bot(group_id, msg)
        except Exception as e:
            logger.print_exc(f"在群聊发送 {group_id} 发送 {region} MasterData更新通知失败")
            continue


# 广告奖励推送
@repeat_with_interval(5, '广告奖励推送', logger)
async def msr_auto_push():
    for region in ALL_SERVER_REGIONS:
        region_name = get_region_name(region)
        ctx = SekaiHandlerContext.from_region(region)

        update_time_url = get_gameapi_config(ctx).ad_result_update_time_api_url
        result_url = get_gameapi_config(ctx).ad_result_api_url
        if not update_time_url or not result_url: continue
        if region not in ad_result_sub.regions: continue

        # 获取订阅的用户列表
        qids = list(set([qid for qid, gid in ad_result_sub.get_all_gid_uid(region)]))
        uids = set()
        for qid in qids:
            for i in range(get_player_bind_count(ctx, qid)):
                try:
                    if uid := get_player_bind_id(ctx, qid, index=i):
                        uids.add(uid)
                except:
                    pass
        if not uids: continue

        # 获取广告奖励更新时间
        try:
            update_times = await request_gameapi(update_time_url)
        except Exception as e:
            logger.warning(f"获取{region_name}广告奖励更新时间失败: {get_exc_desc(e)}")
            continue

        need_push_uids = [] # 需要推送的uid（没有距离太久的）
        for uid in uids:
            update_ts = update_times.get(uid, 0)
            if datetime.now() - datetime.fromtimestamp(update_ts) < timedelta(minutes=10):
                need_push_uids.append(uid)

        tasks = []
                
        for qid, gid in ad_result_sub.get_all_gid_uid(region):
            if check_in_blacklist(qid): continue
            if gid is not None and not gbl.check_id(gid): continue

            for i in range(get_player_bind_count(ctx, qid)):
                ad_result_pushed_time = file_db.get(f"{region}_ad_result_pushed_time", {})

                uid = get_player_bind_id(ctx, qid, index=i)
                if not uid or uid not in need_push_uids:
                    continue

                # 检查这个uid-qid是否已经推送过
                update_ts = int(update_times.get(uid, 0))
                key = f"{uid}-{qid}"
                if key in ad_result_pushed_time:
                    last_push_ts = int(ad_result_pushed_time.get(key, 0))
                    if last_push_ts >= update_ts:
                        continue
                ad_result_pushed_time[key] = update_ts
                file_db.set(f"{region}_ad_result_pushed_time", ad_result_pushed_time)
                
                tasks.append((gid, qid, uid))

        async def push(task):
            gid, qid, uid = task
            try:
                res = await request_gameapi(result_url.format(uid=uid))
                if not res.get('results'):
                    return
                
                if gid is not None:
                    logger.info(f"在 {gid} 中自动推送用户 {qid} 的广告奖励")
                    msg = f"[CQ:at,qq={qid}]的{region_name}广告奖励\n"
                    msg += f"{datetime.fromtimestamp(res['time']).strftime('%Y-%m-%d %H:%M:%S')}\n"
                    msg += "\n".join(res['results'])
                    await send_group_msg_by_bot(gid, msg.strip())
                else:
                    logger.info(f"私聊自动推送用户 {qid} 的广告奖励")
                    msg = f"你的{region_name}广告奖励\n"
                    msg += f"{datetime.fromtimestamp(res['time']).strftime('%Y-%m-%d %H:%M:%S')}\n"
                    msg += "\n".join(res['results'])
                    await send_private_msg_by_bot(qid, msg.strip())
            except Exception as e:
                logger.print_exc(f'自动推送用户 {qid} 的{region_name}广告奖励失败')
                try:
                    if gid is not None:
                        await send_group_msg_by_bot(gid, f"自动推送用户 [CQ:at,qq={qid}] 的{region_name}广告奖励失败: {get_exc_desc(e)}")
                    else:
                        await send_private_msg_by_bot(qid, f"你的{region_name}广告奖励自动推送失败: {get_exc_desc(e)}")
                except: pass

        await batch_gather(*[push(task) for task in tasks])

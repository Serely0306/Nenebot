from __future__ import annotations


def expand_compact_field(compact_data: dict) -> list:
    enum_map = compact_data.get("__ENUM__", {})
    data_fields = {k: v for k, v in compact_data.items() if k != "__ENUM__"}

    if not data_fields:
        return []

    first_field = next(iter(data_fields.values()))
    if not isinstance(first_field, list):
        return []

    count = len(first_field)
    result = []
    for i in range(count):
        obj = {}
        for field_name, values in data_fields.items():
            raw_value = values[i]
            if field_name in enum_map:
                enum_values = enum_map[field_name]
                if isinstance(raw_value, int) and 0 <= raw_value < len(enum_values):
                    obj[field_name] = enum_values[raw_value]
                else:
                    obj[field_name] = raw_value
            else:
                obj[field_name] = raw_value
        result.append(obj)

    return result


RESTORE_LIST_SCHEMAS = {
    "userActionSets": ["id", "status"],
    "userArchiveEventEpisodeStatuses": ["storyType", "episodeId", "status", "isNotSkipped"],
    "userBillingShopItems": ["billingShopItemId", "count", "totalCount", "status"],
    "userChallengeLiveSoloHighScoreRewards": ["characterId", "challengeLiveHighScoreRewardId", "challengeLiveHighScoreStatus"],
    "userChallengeLiveSoloStages": ["challengeLiveStageType", "characterId", "challengeLiveStageId", "rank", "challengeLiveStageStatus", "point"],
    "userCharacterProfileEpisodeStatuses": ["storyType", "episodeId", "status", "isNotSkipped"],
    "userEventArchiveCompleteReadRewards": ["eventStoryId", "isDisplayEventArchiveCompleteReadProgress"],
    "userEventEpisodeStatuses": ["storyType", "episodeId", "status", "isNotSkipped"],
    "userEventExchanges": ["eventExchangeId", "exchangeRemaining"],
    "userHonors": ["honorId", "level", "obtainedAt"],
    "userMaterialExchanges": ["materialExchangeId", "exchangeRemaining"],
    "userMysekaiCharacterTalks": ["mysekaiCharacterTalkId", "isRead"],
    "userReleaseConditions": ["releaseConditionId", "createdAt"],
    "userSekaiEchoCardMissions": ["storyId", "sekaiEchoCardMissionType", "progress"],
    "userSpecialEpisodeStatuses": ["storyType", "episodeId", "status", "isNotSkipped"],
    "userStamps": ["stampId", "obtainedAt"],
    "userUnitEpisodeStatuses": ["storyType", "episodeId", "status", "isNotSkipped"],
    "userWorldBloomSupportDecks": [
        "eventId", "gameCharacterId",
        "member1", "member2", "member3", "member4", "member5", "member6",
        "member7", "member8", "member9", "member10", "member11", "member12",
    ],
    "userWorldBlooms": ["eventId", "gameCharacterId", "rank", "worldBloomChapterPoint", "worldBloomChapterPointUpdateAt"],
}


def _restore_list_field(data: dict, field: str, keys: list[str]):
    items = data.get(field)
    if not isinstance(items, list) or not items or not isinstance(items[0], list):
        return
    result = []
    for item in items:
        if not isinstance(item, list):
            result.append(item)
            continue
        result.append({
            key: item[i] if len(item) > i else None
            for i, key in enumerate(keys)
        })
    data[field] = result


def _restore_card_episodes(episodes):
    if episodes is None:
        return []
    if not isinstance(episodes, list) or not episodes or not isinstance(episodes[0], list):
        return episodes
    result = []
    for episode in episodes:
        item = {
            "cardEpisodeId": episode[0] if len(episode) > 0 else None,
            "scenarioStatus": episode[1] if len(episode) > 1 else None,
            "isNotSkipped": episode[3] if len(episode) > 3 else False,
        }
        if len(episode) > 2 and episode[2] is not None:
            item["scenarioStatusReasons"] = episode[2]
        result.append(item)
    return result


def _restore_user_cards(data: dict):
    cards = data.get("userCards")
    if not isinstance(cards, list) or not cards or not isinstance(cards[0], list):
        return
    result = []
    for card in cards:
        result.append({
            "cardId": card[0] if len(card) > 0 else None,
            "level": card[1] if len(card) > 1 else 1,
            "exp": card[2] if len(card) > 2 else 0,
            "totalExp": card[3] if len(card) > 3 else 0,
            "skillLevel": card[4] if len(card) > 4 else 1,
            "skillExp": card[5] if len(card) > 5 else 0,
            "totalSkillExp": card[6] if len(card) > 6 else 0,
            "masterRank": card[7] if len(card) > 7 else 0,
            "specialTrainingStatus": card[8] if len(card) > 8 else "not_doing",
            "defaultImage": card[9] if len(card) > 9 else "original",
            "duplicateCount": card[10] if len(card) > 10 else 0,
            "createdAt": card[11] if len(card) > 11 else None,
            "episodes": _restore_card_episodes(card[12] if len(card) > 12 else []),
        })
    data["userCards"] = result


def _restore_user_shops(data: dict):
    shops = data.get("userShops")
    if not isinstance(shops, list) or not shops or not isinstance(shops[0], list):
        return
    result = []
    for shop in shops:
        if not isinstance(shop, list):
            result.append(shop)
            continue
        user_shop_items = []
        raw_items = shop[1] if len(shop) > 1 and isinstance(shop[1], list) else []
        for item in raw_items:
            if not isinstance(item, list):
                user_shop_items.append(item)
                continue
            user_shop_items.append({
                "shopItemId": item[0] if len(item) > 0 else None,
                "level": item[1] if len(item) > 1 else None,
                "status": item[2] if len(item) > 2 else None,
            })
        result.append({
            "shopId": shop[0] if len(shop) > 0 else None,
            "userShopItems": user_shop_items,
        })
    data["userShops"] = result


def _restore_user_areas(data: dict):
    areas = data.get("userAreas")
    if not isinstance(areas, list) or not areas or not isinstance(areas[0], dict):
        return
    for area in areas:
        action_sets = area.get("actionSets")
        if not isinstance(action_sets, list) or not action_sets or not isinstance(action_sets[0], list):
            continue
        area["actionSets"] = [
            {
                "id": item[0] if len(item) > 0 else None,
                "status": item[1] if len(item) > 1 else None,
            }
            for item in action_sets
        ]


def _restore_user_virtual_shops(data: dict):
    shops = data.get("userVirtualShops")
    if not isinstance(shops, list) or not shops or not isinstance(shops[0], list):
        return
    result = []
    for shop in shops:
        if not isinstance(shop, list):
            result.append(shop)
            continue
        user_virtual_shop_items = []
        raw_items = shop[1] if len(shop) > 1 and isinstance(shop[1], list) else []
        for item in raw_items:
            if not isinstance(item, list):
                user_virtual_shop_items.append(item)
                continue
            user_virtual_shop_items.append({
                "virtualShopId": item[0] if len(item) > 0 else None,
                "virtualShopItemId": item[1] if len(item) > 1 else None,
                "status": item[2] if len(item) > 2 else None,
            })
        result.append({
            "virtualShopId": shop[0] if len(shop) > 0 else None,
            "userVirtualShopItems": user_virtual_shop_items,
        })
    data["userVirtualShops"] = result


def restore_suite_fields(data: dict) -> dict:
    if not isinstance(data, dict):
        return data
    for field, keys in RESTORE_LIST_SCHEMAS.items():
        _restore_list_field(data, field, keys)
    _restore_user_cards(data)
    _restore_user_areas(data)
    _restore_user_shops(data)
    _restore_user_virtual_shops(data)
    return data


def process_suite_compact(data: dict) -> dict:
    result = {}
    for key, value in data.items():
        if key.startswith("compact") and isinstance(value, dict):
            expanded_key = key[len("compact") :]
            expanded_key = expanded_key[0].lower() + expanded_key[1:]
            expanded = expand_compact_field(value)
            result[expanded_key] = expanded
            print(f"  [compact] {key} -> {expanded_key}: {len(expanded)} 条")
        else:
            result[key] = value
    return restore_suite_fields(result)


def extract_suite_user_id(data: dict) -> str | None:
    if not isinstance(data, dict):
        return None

    user_gamedata = data.get("userGamedata")
    if isinstance(user_gamedata, list) and len(user_gamedata) > 0:
        uid = user_gamedata[0].get("userId") if isinstance(user_gamedata[0], dict) else None
        if uid is not None:
            return str(uid)
    elif isinstance(user_gamedata, dict):
        uid = user_gamedata.get("userId")
        if uid is not None:
            return str(uid)

    compact_gamedata = data.get("compactUserGamedata")
    if isinstance(compact_gamedata, dict):
        user_ids = compact_gamedata.get("userId")
        if isinstance(user_ids, list) and len(user_ids) > 0:
            return str(user_ids[0])

    uid = data.get("userId")
    if uid is not None:
        return str(uid)

    return None

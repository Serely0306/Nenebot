"""Adapt uploaded Suite data for features that only need account metadata."""

from copy import deepcopy
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit


BASIC_PROFILE_FIELDS = (
    "userGamedata", "userDecks", "userCards", "userProfile",
    "userProfileHonors", "userCharacters", "userMusicResults", "upload_time",
)


def build_upload_data_url(template, uid, mode, filter_keys=None):
    parts = urlsplit(template.format(uid=uid))
    query = dict(parse_qsl(parts.query, keep_blank_values=True))
    field_param = "key" if "key" in query and "filter" not in query else "filter"
    keys = sorted(filter_keys) if isinstance(filter_keys, (set, frozenset)) else filter_keys
    if keys:
        query[field_param] = ",".join(dict.fromkeys(keys))
    query["mode"] = mode
    return urlunsplit(parts._replace(query=urlencode(query)))


def build_basic_profile_from_suite(payload, uid):
    payload = payload if isinstance(payload, dict) else {}
    gamedata = payload.get("userGamedata") or {}
    if not isinstance(gamedata, dict):
        gamedata = {}
    if gamedata.get("userId") is not None and str(gamedata["userId"]) != str(uid):
        raise ValueError("Suite userId does not match the requested account")

    profile = {
        "user": {"userId": uid, "name": "?", "rank": 0, **gamedata},
        "userDeck": {},
        "userCards": deepcopy(payload.get("userCards") or []),
        "userProfile": deepcopy(payload.get("userProfile") or {}),
        "userProfileHonors": deepcopy(payload.get("userProfileHonors") or []),
        "userCharacters": deepcopy(payload.get("userCharacters") or []),
        "offline_metadata": True,
    }
    deck_id = gamedata.get("deck")
    decks = [deck for deck in payload.get("userDecks", []) if isinstance(deck, dict)]
    current = next((deck for deck in decks if deck.get("deckId") == deck_id), None)
    if current is not None:
        profile["userDeck"] = deepcopy(current)
    if payload.get("upload_time") is not None:
        uploaded_at = float(payload["upload_time"])
        if uploaded_at < 1e11:
            uploaded_at *= 1000
        elif uploaded_at > 1e14:
            uploaded_at /= 1000
        profile["update_time"] = uploaded_at

    best_results = {}
    for item in payload.get("userMusicResults", []):
        if not isinstance(item, dict):
            continue
        difficulty = item.get("musicDifficultyType") or item.get("musicDifficulty")
        key = (item.get("musicId"), difficulty)
        if None in key:
            continue
        level = 0
        if item.get("playResult") not in (None, "not_clear"):
            level = 1
        if item.get("fullComboFlg"):
            level = 2
        if item.get("fullPerfectFlg"):
            level = 3
        best_results[key] = max(level, best_results.get(key, 0))
    counts = []
    for difficulty in ("easy", "normal", "hard", "expert", "master", "append"):
        levels = [level for (_, diff), level in best_results.items() if diff == difficulty]
        counts.append({
            "musicDifficultyType": difficulty,
            "liveClear": sum(level >= 1 for level in levels),
            "fullCombo": sum(level >= 2 for level in levels),
            "allPerfect": sum(level >= 3 for level in levels),
        })
    profile["userMusicDifficultyClearCount"] = counts
    return profile

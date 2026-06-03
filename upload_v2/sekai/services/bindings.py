from __future__ import annotations

import json

from core.runtime import LUNABOT_BASE, PROFILE_DB_PATH, REGION_NAMES, VALID_REGIONS

__all__ = [
    "VALID_REGIONS",
    "REGION_NAMES",
    "LUNABOT_BASE",
    "PROFILE_DB_PATH",
    "load_profile_db",
    "mask_game_id",
    "get_bind_info",
    "ensure_bound_account",
]


def load_profile_db() -> dict:
    try:
        with PROFILE_DB_PATH.open("r", encoding="utf-8") as fh:
            return json.load(fh)
    except FileNotFoundError:
        return {"bind_list": {}, "main_bind_list": {}}
    except Exception:
        return {"bind_list": {}, "main_bind_list": {}}


def mask_game_id(game_id: str, keep: int = 6) -> str:
    game_id = str(game_id)
    if len(game_id) <= keep:
        return game_id
    return "*" * (len(game_id) - keep) + game_id[-keep:]


def get_bind_info(qq_id: str, region: str) -> dict:
    db = load_profile_db()
    bind_list = db.get("bind_list", {}).get(region, {})
    main_bind_list = db.get("main_bind_list", {}).get(region, {})

    qq_id = str(qq_id)
    bound_ids = bind_list.get(qq_id, [])
    if isinstance(bound_ids, str):
        bound_ids = [bound_ids]

    main_id = main_bind_list.get(qq_id, bound_ids[0] if bound_ids else None)
    return {
        "qq_id": qq_id,
        "region": region,
        "bound_ids": [str(item) for item in bound_ids],
        "main_id": str(main_id) if main_id is not None else None,
        "has_binding": len(bound_ids) > 0,
    }


def ensure_bound_account(qq_id: str, region: str, game_id: str) -> bool:
    bind_info = get_bind_info(qq_id, region)
    return str(game_id) in bind_info["bound_ids"]

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
    return result


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

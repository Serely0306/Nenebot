from __future__ import annotations

import copy
import hashlib
import json
import sqlite3
import time
from pathlib import Path

from sekai.services.suite import normalize_mysekai_harvest_maps


def event_database(base: Path, region: str) -> Path:
    return base / region / "mysekai_birthday" / "events.sqlite3"


def _connect(path: Path):
    path.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(path, timeout=30)
    conn.execute("""CREATE TABLE IF NOT EXISTS events (
        sequence INTEGER PRIMARY KEY AUTOINCREMENT,
        uid TEXT NOT NULL, event_id TEXT NOT NULL,
        event_time INTEGER NOT NULL, received_at INTEGER NOT NULL,
        payload TEXT NOT NULL, snapshot TEXT NOT NULL,
        UNIQUE(uid, event_id)
    )""")
    conn.execute("CREATE INDEX IF NOT EXISTS events_uid_sequence ON events(uid, sequence)")
    return conn


def game_time(data: dict) -> int:
    resources = data.get("updatedResources", {})
    return int(resources.get("now") or data.get("now") or data.get("upload_time") or 0)


def prepare_delivery(data: dict) -> dict:
    result = normalize_mysekai_harvest_maps(copy.deepcopy(data))
    resources = result.get("updatedResources")
    if not isinstance(resources, dict):
        raise ValueError("Birthday delivery is missing updatedResources")
    parties = resources.get("userBirthdayParties") or []
    party_id = str(result.get("birthday_party_id") or "")
    if not party_id and len(parties) == 1:
        party_id = str(parties[0].get("birthdayPartyId", ""))
    if not party_id.isdecimal() or int(party_id) <= 0:
        raise ValueError("Birthday delivery requires a valid birthday_party_id")
    if parties and not any(str(p.get("birthdayPartyId")) == party_id for p in parties):
        raise ValueError("Birthday party id does not match response")
    result["birthday_party_id"] = party_id
    if result.get("isRefreshed") is not True:
        return result
    maps = resources.get("userMysekaiHarvestMaps")
    if not isinstance(maps, list) or not maps:
        raise ValueError("Birthday refresh is missing harvest maps")
    for site in maps:
        if not isinstance(site, dict) or "mysekaiSiteId" not in site:
            raise ValueError("Invalid birthday harvest map")
        for key in ("userMysekaiSiteHarvestFixtures", "userMysekaiSiteHarvestResourceDrops"):
            if not isinstance(site.get(key), list) or not all(isinstance(row, dict) for row in site[key]):
                raise ValueError("Invalid birthday harvest rows")
    # Event identity must not depend on client clocks or uploader source labels.
    event_time = resources.get("now")
    if not isinstance(event_time, int) or event_time <= 0:
        raise ValueError("Birthday refresh requires the game response timestamp")
    result["upload_time"] = event_time
    return result


def merge_delivery(current: dict, delivery: dict) -> dict:
    snapshot = normalize_mysekai_harvest_maps(copy.deepcopy(current))
    resources = snapshot.setdefault("updatedResources", {})
    incoming = delivery["updatedResources"]
    resources.update(copy.deepcopy(incoming))
    # Merge lists with identities; an update need not contain all parties/materials.
    for key, identity in (("userBirthdayParties", "birthdayPartyId"), ("userMaterials", "materialId")):
        if key in incoming:
            previous = current.get("updatedResources", {}).get(key, [])
            items = {item[identity]: copy.deepcopy(item) for item in previous if identity in item}
            items.update({item[identity]: copy.deepcopy(item) for item in incoming[key] if identity in item})
            resources[key] = list(items.values())
    for key in ("droppedRewardCount", "obtainedDeliveryTotalRewards", "isRefreshed", "source", "local_source"):
        if key in delivery:
            snapshot[key] = copy.deepcopy(delivery[key])
    snapshot["upload_time"] = game_time(delivery)
    snapshot["birthday_party_id"] = delivery["birthday_party_id"]
    snapshot["mysekai_update_kind"] = "birthday"
    snapshot["full_upload_time"] = current.get("full_upload_time", current.get("upload_time", 0))
    return snapshot


def record_delivery(base: Path, region: str, uid: str, delivery: dict, current: dict) -> dict | None:
    if delivery.get("isRefreshed") is not True:
        return None
    identity = {k: delivery[k] for k in (
        "birthday_party_id", "updatedResources", "droppedRewardCount", "obtainedDeliveryTotalRewards", "isRefreshed"
    ) if k in delivery}
    event_id = hashlib.sha256(json.dumps(identity, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    snapshot = merge_delivery(current, delivery)
    snapshot["birthday_event_id"] = event_id
    conn = _connect(event_database(base, region))
    try:
        with conn:
            conn.execute(
                "INSERT OR IGNORE INTO events(uid,event_id,event_time,received_at,payload,snapshot) VALUES (?,?,?,?,?,?)",
                (uid, event_id, game_time(delivery), int(time.time() * 1000), json.dumps(delivery), json.dumps(snapshot)),
            )
            row = conn.execute("SELECT sequence,snapshot FROM events WHERE uid=? AND event_id=?", (uid, event_id)).fetchone()
        return {"sequence": row[0], "event_id": event_id, "snapshot": json.loads(row[1])}
    finally:
        conn.close()


def read_events(base: Path, region: str, uid: str, after: int | None, limit: int) -> dict:
    path = event_database(base, region)
    if not path.exists():
        return {"events": [], "next_cursor": after or 0, "has_more": False}
    conn = _connect(path)
    try:
        # New subscribers start here, without replaying their historical deliveries.
        if after is None:
            tail = conn.execute("SELECT COALESCE(MAX(sequence),0) FROM events WHERE uid=?", (uid,)).fetchone()[0]
            return {"events": [], "next_cursor": tail, "has_more": False}
        rows = conn.execute(
            "SELECT sequence,event_id,event_time,payload,snapshot FROM events WHERE uid=? AND sequence>? ORDER BY sequence LIMIT ?",
            (uid, after, limit + 1),
        ).fetchall()
        events = [{"sequence": seq, "event_id": eid, "event_time": stamp, "user_id": uid,
                   "birthday_party_id": json.loads(raw)["birthday_party_id"], "snapshot": json.loads(snapshot)}
                  for seq, eid, stamp, raw, snapshot in rows[:limit]]
        return {"events": events, "next_cursor": events[-1]["sequence"] if events else after, "has_more": len(rows) > limit}
    finally:
        conn.close()

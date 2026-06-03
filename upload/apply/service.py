from __future__ import annotations

import json
import os
import secrets
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

DATA_DIR = Path(__file__).resolve().parent.parent / "data"
DB_PATH = DATA_DIR / "applications.json"
_LOCK = threading.Lock()


def _ensure_dir() -> None:
    DATA_DIR.mkdir(parents=True, exist_ok=True)


def _read_all() -> list[dict]:
    _ensure_dir()
    with _LOCK:
        try:
            with DB_PATH.open("r", encoding="utf-8") as fh:
                data = json.load(fh)
            if isinstance(data, list):
                return data
            if isinstance(data, dict) and "records" in data:
                return data["records"]
            return []
        except (FileNotFoundError, json.JSONDecodeError):
            return []


def _write_all(records: list[dict]) -> None:
    _ensure_dir()
    meta = _read_meta()
    with _LOCK:
        with DB_PATH.open("w", encoding="utf-8") as fh:
            json.dump({"meta": meta, "records": records}, fh, ensure_ascii=False, indent=2)


def _read_meta() -> dict:
    _ensure_dir()
    with _LOCK:
        try:
            with DB_PATH.open("r", encoding="utf-8") as fh:
                data = json.load(fh)
            if isinstance(data, dict) and "meta" in data:
                return data["meta"]
            return {}
        except (FileNotFoundError, json.JSONDecodeError):
            return {}


def _write_meta(meta: dict) -> None:
    records = _read_all()
    _ensure_dir()
    with _LOCK:
        with DB_PATH.open("w", encoding="utf-8") as fh:
            json.dump({"meta": meta, "records": records}, fh, ensure_ascii=False, indent=2)


def is_ip_blocked(ip: str) -> bool:
    meta = _read_meta()
    return ip in meta.get("ip_blacklist", [])


def create_application(group_id: str, member_count: int,
                       purpose: str, applicant: str,
                       client_ip: str = "") -> dict:
    records = _read_all()
    record = {
        "id": secrets.token_hex(4),
        "group_id": str(group_id).strip(),
        "group_name": "",
        "member_count": int(member_count),
        "purpose": str(purpose).strip(),
        "applicant": str(applicant).strip(),
        "applicant_nickname": "",
        "client_ip": str(client_ip).strip(),
        "verified": None,
        "verified_at": None,
        "verification_note": None,
        "status": "pending",
        "admin_note": "",
        "visible": True,
        "created_at": datetime.now(timezone.utc).isoformat(),
        "reviewed_at": None,
    }
    records.insert(0, record)
    _write_all(records)
    return record


def get_applications(status: Optional[str] = None,
                    include_deleted: bool = False) -> list[dict]:
    records = _read_all()
    if status:
        records = [r for r in records if r.get("status") == status]
    if not include_deleted:
        records = [r for r in records if r.get("visible", True)]
    return records


def get_application_by_id(app_id: str) -> Optional[dict]:
    for r in _read_all():
        if r["id"] == app_id:
            return r
    return None


def get_applications_by_applicant(applicant: str) -> list[dict]:
    applicant = str(applicant).strip()
    return [r for r in _read_all()
            if r.get("applicant") == applicant and r.get("visible", True)]


def update_status(app_id: str, new_status: str,
                  admin_note: str = "") -> Optional[dict]:
    if new_status not in ("approved", "rejected", "pending"):
        return None
    records = _read_all()
    for r in records:
        if r["id"] == app_id:
            r["status"] = new_status
            r["admin_note"] = str(admin_note).strip()
            r["reviewed_at"] = datetime.now(timezone.utc).isoformat()
            _write_all(records)
            return r
    return None


def update_note(app_id: str, admin_note: str) -> Optional[dict]:
    records = _read_all()
    for r in records:
        if r["id"] == app_id:
            r["admin_note"] = str(admin_note).strip()
            _write_all(records)
            return r
    return None


def get_meta() -> dict:
    return _read_meta()


def add_ip_to_blacklist(ip: str) -> bool:
    ip = ip.strip()
    if not ip:
        return False
    meta = _read_meta()
    bl = meta.get("ip_blacklist", [])
    if ip in bl:
        return False
    meta["ip_blacklist"] = bl + [ip]
    _write_meta(meta)
    return True


def remove_ip_from_blacklist(ip: str) -> bool:
    ip = ip.strip()
    if not ip:
        return False
    meta = _read_meta()
    bl = meta.get("ip_blacklist", [])
    if ip not in bl:
        return False
    meta["ip_blacklist"] = [x for x in bl if x != ip]
    _write_meta(meta)
    return True


def is_group_pending(group_id: str) -> bool:
    group_id = str(group_id).strip()
    for r in _read_all():
        if (r.get("group_id") == group_id
                and r.get("status") == "pending"
                and r.get("visible", True)):
            return True
    return False


def delete_record(app_id: str) -> Optional[dict]:
    records = _read_all()
    for r in records:
        if r["id"] == app_id:
            r["visible"] = False
            _write_all(records)
            return r
    return None


def get_approved_applicants() -> set[str]:
    return {r["applicant"] for r in _read_all()
            if r.get("status") == "approved" and r.get("visible", True)}

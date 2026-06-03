from __future__ import annotations

from pathlib import Path

import requests
import yaml
from flask import Blueprint, jsonify, request, send_from_directory

import threading
from collections import defaultdict
from time import monotonic

_rate_lock = threading.Lock()

from apply.service import (
    DuplicatePendingGroupError,
    StorageLockTimeoutError,
    add_ip_to_blacklist,
    create_application,
    delete_record,
    get_application_by_id,
    get_applications,
    get_applications_by_applicant,
    get_meta,
    is_ip_blocked,
    remove_ip_from_blacklist,
    update_note,
    update_status,
)

_ip_limits: defaultdict[str, list[float]] = defaultdict(list)
_applicant_limits: defaultdict[str, list[float]] = defaultdict(list)

IP_WINDOW = 60.0       # seconds
IP_MAX_REQUESTS = 5
APPLICANT_WINDOW = 60.0
APPLICANT_MAX_REQUESTS = 2


def _check_rate_limit(store: defaultdict[str, list[float]], key: str,
                      window: float, max_req: int) -> bool:
    """Return True if rate limit exceeded."""
    now = monotonic()
    with _rate_lock:
        store[key] = [t for t in store[key] if now - t < window]
        if len(store[key]) >= max_req:
            return True
        store[key].append(now)
    return False


APPLY_DIR = Path(__file__).resolve().parent
PAGES_DIR = APPLY_DIR / "pages"
STATIC_DIR = APPLY_DIR / "static"
CONFIG_PATH = APPLY_DIR.parent / "config.yaml"

bp = Blueprint("apply", __name__)


def _get_client_ip() -> str:
    return (request.remote_addr or "unknown").strip()


def _build_public_status(record: dict) -> dict:
    member_count = record.get("member_count", 0)
    try:
        member_count = int(member_count)
    except (TypeError, ValueError):
        member_count = 0
    return {
        "group_id": str(record.get("group_id", "")).strip(),
        "purpose": str(record.get("purpose", "")).strip(),
        "member_count": member_count,
        "admin_note": str(record.get("admin_note", "")).strip(),
        "status": str(record.get("status", "pending")).strip() or "pending",
    }


def _service_busy_response():
    return jsonify({"error": "系统繁忙，请稍后重试"}), 503


def _get_admin_token() -> str:
    try:
        with CONFIG_PATH.open("r", encoding="utf-8") as fh:
            raw = yaml.safe_load(fh) or {}
        return str(raw.get("apply", {}).get("admin_token", "change-me"))
    except Exception:
        return "change-me"


def _check_token() -> bool:
    token = request.headers.get("X-Admin-Token", "")
    expected = _get_admin_token()
    return token and token == expected


def _require_token():
    from flask import abort
    if not _check_token():
        abort(403)


def _get_notify_url() -> str:
    try:
        with CONFIG_PATH.open("r", encoding="utf-8") as fh:
            raw = yaml.safe_load(fh) or {}
        return str(raw.get("apply", {}).get("notify_url", ""))
    except Exception:
        return ""


def _notify_onebot(app_id: str) -> None:
    url = _get_notify_url()
    if not url:
        return
    try:
        requests.post(url, json={"app_id": app_id}, timeout=2)
    except Exception:
        pass


# ── 页面路由 ──

@bp.route("/apply")
def apply_page():
    return send_from_directory(str(PAGES_DIR), "apply.html")


@bp.route("/review")
def review_page():
    return send_from_directory(str(PAGES_DIR), "review.html")


@bp.route("/apply.js")
def apply_js():
    return send_from_directory(str(STATIC_DIR), "apply.js")


@bp.route("/review.js")
def review_js():
    return send_from_directory(str(STATIC_DIR), "review.js")


# ── 公开 API ──

@bp.route("/api/apply", methods=["POST"])
def api_submit():
    client_ip = _get_client_ip()

    if _check_rate_limit(_ip_limits, client_ip, IP_WINDOW, IP_MAX_REQUESTS):
        return jsonify({"error": "请求过于频繁，请稍后再试"}), 429

    if is_ip_blocked(client_ip):
        return jsonify({"error": "禁止提交"}), 403

    data = request.get_json(silent=True)
    if not data:
        return jsonify({"error": "请提供 JSON 数据"}), 400

    group_id = str(data.get("group_id", "")).strip()
    purpose = str(data.get("purpose", "")).strip()
    applicant = str(data.get("applicant", "")).strip()

    errors = []
    if not group_id or not group_id.isdigit():
        errors.append("QQ群号格式不正确")
    if not purpose:
        errors.append("拉群目的不能为空")
    if not applicant or not applicant.isdigit():
        errors.append("申请人QQ号格式不正确")
    if errors:
        return jsonify({"error": "; ".join(errors)}), 400

    if _check_rate_limit(_applicant_limits, applicant, APPLICANT_WINDOW, APPLICANT_MAX_REQUESTS):
        return jsonify({"error": "请求过于频繁，请稍后再试"}), 429

    try:
        record = create_application(group_id, 0, purpose, applicant, client_ip)
    except DuplicatePendingGroupError:
        return jsonify({"error": "该群正在申请中，请等待审核结果"}), 409
    except StorageLockTimeoutError:
        return jsonify({"error": "系统繁忙，请稍后重试"}), 503
    _notify_onebot(record["id"])
    return jsonify({"success": True, "id": record["id"]})


@bp.route("/api/apply/status", methods=["GET"])
def api_status():
    applicant = request.args.get("applicant", "").strip()
    if not applicant or not applicant.isdigit():
        return jsonify({"error": "QQ号格式不正确"}), 400
    results = get_applications_by_applicant(applicant)
    return jsonify([_build_public_status(record) for record in results])


# ── 管理端 API ──

@bp.route("/api/apply/auth", methods=["POST"])
def api_auth():
    data = request.get_json(silent=True)
    if not data:
        return "", 403
    token = str(data.get("token", "")).strip()
    if token and token == _get_admin_token():
        return jsonify({"success": True})
    return "", 403


@bp.route("/api/apply/list", methods=["GET"])
def api_list():
    _require_token()
    status = request.args.get("status", "").strip()
    show_deleted = request.args.get("show_deleted", "0") == "1"
    records = get_applications(status if status else None,
                               include_deleted=show_deleted)
    return jsonify(records)


@bp.route("/api/apply/<app_id>/approve", methods=["POST"])
def api_approve(app_id):
    _require_token()
    data = request.get_json(silent=True) or {}
    note = str(data.get("admin_note", "")).strip()
    try:
        record = update_status(app_id, "approved", note)
    except StorageLockTimeoutError:
        return _service_busy_response()
    if record is None:
        existing = get_application_by_id(app_id)
        if existing is None:
            return jsonify({"error": "申请不存在"}), 404
        return jsonify({"error": "申请已审核，不可重复操作"}), 400
    return jsonify(record)


@bp.route("/api/apply/<app_id>/reject", methods=["POST"])
def api_reject(app_id):
    _require_token()
    data = request.get_json(silent=True) or {}
    note = str(data.get("admin_note", "")).strip()
    try:
        record = update_status(app_id, "rejected", note)
    except StorageLockTimeoutError:
        return _service_busy_response()
    if record is None:
        existing = get_application_by_id(app_id)
        if existing is None:
            return jsonify({"error": "申请不存在"}), 404
        return jsonify({"error": "申请已审核，不可重复操作"}), 400
    return jsonify(record)


@bp.route("/api/apply/<app_id>/revoke", methods=["POST"])
def api_revoke(app_id):
    _require_token()
    data = request.get_json(silent=True) or {}
    note = str(data.get("admin_note", "")).strip()
    try:
        record = update_status(app_id, "pending", note)
    except StorageLockTimeoutError:
        return _service_busy_response()
    if record is None:
        existing = get_application_by_id(app_id)
        if existing is None:
            return jsonify({"error": "申请不存在"}), 404
        return jsonify({"error": "当前状态不可撤销"}), 400
    return jsonify(record)


@bp.route("/api/apply/<app_id>/note", methods=["POST"])
def api_update_note(app_id):
    _require_token()
    data = request.get_json(silent=True) or {}
    note = str(data.get("admin_note", "")).strip()
    try:
        record = update_note(app_id, note)
    except StorageLockTimeoutError:
        return _service_busy_response()
    if record is None:
        return jsonify({"error": "申请不存在"}), 404
    return jsonify(record)


@bp.route("/api/apply/<app_id>/delete", methods=["POST"])
def api_delete(app_id):
    _require_token()
    try:
        record = delete_record(app_id)
    except StorageLockTimeoutError:
        return _service_busy_response()
    if record is None:
        return jsonify({"error": "申请不存在"}), 404
    return jsonify(record)


# ── Meta / IP 黑名单管理 ──

@bp.route("/api/apply/meta", methods=["GET"])
def api_get_meta():
    _require_token()
    return jsonify(get_meta())


@bp.route("/api/apply/meta/ip-blacklist", methods=["POST"])
def api_ip_blacklist():
    _require_token()
    data = request.get_json(silent=True) or {}
    action = str(data.get("action", "")).strip()
    ip = str(data.get("ip", "")).strip()
    if not ip:
        return jsonify({"error": "IP 不能为空"}), 400
    try:
        if action == "add":
            ok = add_ip_to_blacklist(ip)
            return jsonify({"success": ok, "message": "已在列表中" if not ok else "已添加"})
        elif action == "remove":
            ok = remove_ip_from_blacklist(ip)
            return jsonify({"success": ok, "message": "不在列表中" if not ok else "已移除"})
    except StorageLockTimeoutError:
        return _service_busy_response()
    return jsonify({"error": "无效操作，请使用 add 或 remove"}), 400

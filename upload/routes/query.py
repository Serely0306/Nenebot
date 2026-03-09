from __future__ import annotations

import json

import requests as http_requests
from flask import jsonify, request

from core.runtime import LUNABOT_DATA_BASE, PROFILE_DB_PATH, REGION_NAMES, UPLOAD_CONFIG, VALID_REGIONS
from services.crypto import DECRYPT_AVAILABLE
from services.data import load_and_filter_json
from services.profile import get_bind_info, mask_game_id


def register_query_routes(app):
    @app.route("/api/query_binding", methods=["POST"])
    def query_binding():
        data = request.get_json()
        if not data:
            return jsonify({"error": "请提供 JSON 数据"}), 400

        qq_id = data.get("qq_id", "").strip()
        region = data.get("region", "jp").lower()

        if not qq_id:
            return jsonify({"error": "请输入 QQ 号"}), 400
        if not qq_id.isdigit():
            return jsonify({"error": "QQ 号格式不正确"}), 400
        if region not in VALID_REGIONS:
            return jsonify({"error": f"不支持的区服: {region}"}), 400

        bind_info = get_bind_info(qq_id, region)
        if not bind_info["has_binding"]:
            return jsonify(
                {
                    "success": False,
                    "error": f"该 QQ 号在{REGION_NAMES.get(region, region)}没有绑定任何游戏账号",
                    "qq_id": qq_id,
                    "region": region,
                }
            )

        accounts = []
        for index, game_id in enumerate(bind_info["bound_ids"]):
            accounts.append(
                {
                    "index": index + 1,
                    "game_id": game_id,
                    "display_id": mask_game_id(game_id),
                    "is_main": game_id == bind_info["main_id"],
                }
            )

        return jsonify(
            {
                "success": True,
                "qq_id": qq_id,
                "region": region,
                "region_name": REGION_NAMES.get(region, region),
                "accounts": accounts,
                "main_id": bind_info["main_id"],
                "main_display_id": mask_game_id(bind_info["main_id"]) if bind_info["main_id"] else None,
            }
        )

    @app.route("/api/mysekai/<region>/<uid>", methods=["GET"])
    def get_mysekai_data(region, uid):
        region = region.lower()
        if region not in VALID_REGIONS:
            return jsonify({"error": f"不支持的区服: {region}"}), 404

        mode = request.args.get("mode", "local")
        filter_str = request.args.get("filter", "")
        filter_keys = [k for k in filter_str.split(",") if k]

        local_err = None
        file_path = LUNABOT_DATA_BASE / region / "mysekai" / f"{uid}.json"
        if mode in ["local", "auto", "latest"]:
            if file_path.exists():
                try:
                    data = load_and_filter_json(file_path, filter_keys)
                    return jsonify(data)
                except Exception as exc:
                    return jsonify({"error": f"读取数据失败: {exc}"}), 500
            local_err = "文件不存在"
            if mode == "local":
                return jsonify({"local_err": local_err}), 404

        return jsonify({"local_err": local_err or "文件不存在"}), 404

    @app.route("/api/suite/<region>/<uid>", methods=["GET"])
    def get_suite_data(region, uid):
        region = region.lower()
        if region not in VALID_REGIONS:
            return jsonify({"error": f"不支持的区服: {region}"}), 404

        mode = request.args.get("mode", "auto")
        filter_str = request.args.get("filter", "") or request.args.get("key", "")
        filter_keys = [k for k in filter_str.rstrip(",").split(",") if k]

        local_err = None
        haruki_err = None
        local_path = LUNABOT_DATA_BASE / region / "suite" / f"{uid}.json"
        haruki_unsupported_keys = {"userChargedCurrency", "userBoostItems"}

        def get_local_data():
            nonlocal local_err
            if not local_path.exists():
                local_err = "文件不存在"
                return None
            try:
                return load_and_filter_json(local_path, filter_keys)
            except Exception as exc:
                local_err = str(exc)
                return None

        def get_haruki_data():
            nonlocal haruki_err
            haruki_url = f"https://toolbox-api-direct.haruki.seiunx.com/public/{region}/suite/{uid}"
            haruki_filter_keys = [key for key in filter_keys if key not in haruki_unsupported_keys]
            if haruki_filter_keys:
                haruki_url += f"?key={','.join(haruki_filter_keys)}"
            try:
                resp = http_requests.get(haruki_url, timeout=15)
                if resp.ok:
                    data = resp.json()
                    if data is not None:
                        data["source"] = "Haruki ToolBox"
                        return data
                    haruki_err = "返回数据为空"
                    return None
                if resp.status_code == 404:
                    haruki_err = "请检查是否已绑定Haruki工具箱并上传数据"
                elif resp.status_code == 403:
                    haruki_err = "请在Haruki工具箱中设置允许公开API访问"
                else:
                    haruki_err = f"HTTP {resp.status_code}"
                return None
            except Exception as exc:
                haruki_err = str(exc)
                return None

        if mode == "local":
            data = get_local_data()
            if data:
                return jsonify(data)
            return jsonify({"local_err": local_err}), 404

        if mode == "haruki":
            data = get_haruki_data()
            if data:
                return jsonify(data)
            return jsonify({"haruki_err": haruki_err}), 404

        if mode == "latest":
            local_data = get_local_data()
            haruki_data = get_haruki_data()
            if local_data and haruki_data:
                local_time = local_data.get("upload_time", 0)
                haruki_time = haruki_data.get("upload_time", 0)
                if haruki_time < 10000000000:
                    haruki_time *= 1000
                return jsonify(local_data if local_time >= haruki_time else haruki_data)
            if local_data:
                return jsonify(local_data)
            if haruki_data:
                return jsonify(haruki_data)
            return jsonify({"local_err": local_err, "haruki_err": haruki_err}), 404

        local_data = get_local_data()
        if local_data:
            return jsonify(local_data)
        haruki_data = get_haruki_data()
        if haruki_data:
            return jsonify(haruki_data)
        return jsonify({"local_err": local_err, "haruki_err": haruki_err}), 404

    @app.route("/api/mysekai/<region>/upload_times", methods=["GET", "POST"])
    def get_mysekai_upload_times(region):
        region = region.lower()
        if region not in VALID_REGIONS:
            return jsonify({"error": f"不支持的区服: {region}"}), 404

        uid_modes = request.get_json()
        if not uid_modes:
            return jsonify([])

        upload_times = []
        for uid, mode in uid_modes:
            file_path = LUNABOT_DATA_BASE / region / "mysekai" / f"{uid}.json"
            if file_path.exists():
                try:
                    with file_path.open("r", encoding="utf-8") as fh:
                        data = json.load(fh)
                    upload_times.append(data.get("upload_time", 0))
                except Exception:
                    upload_times.append(0)
            else:
                upload_times.append(0)
        return jsonify(upload_times)

    @app.route("/api/mysekai/<region>/msr_sub", methods=["PUT"])
    def update_msr_sub(region):
        region = region.lower()
        uid_modes = request.get_json()
        print(f"更新 {region} MSR 订阅: {len(uid_modes or [])} 个用户")
        return jsonify({"success": True, "count": len(uid_modes or [])})

    @app.route("/status", methods=["GET"])
    def status():
        return jsonify(
            {
                "status": "ok",
                "version": "2.0.0",
                "supported_regions": VALID_REGIONS,
                "decrypt_available": DECRYPT_AVAILABLE,
                "profile_db_available": PROFILE_DB_PATH.exists(),
                "msr_available": True,
                "msr_access_key_configured": bool(
                    UPLOAD_CONFIG.msr.access_key and UPLOAD_CONFIG.msr.access_key != "change-me"
                ),
            }
        )

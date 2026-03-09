from __future__ import annotations

import io
import time

from flask import jsonify, request, send_file

from core.bindings import ensure_bound_account
from core.runtime import VALID_REGIONS, get_msr_repo, get_upload_config
from services.msr_service import MsrQueryError, render_msr_image


def register_msr_routes(app):
    @app.route("/api/msr/validate_access_key", methods=["POST"])
    def validate_msr_access_key():
        data = request.get_json()
        if not data:
            return jsonify({"error": "请提供 JSON 数据"}), 400

        config = get_upload_config()
        access_key = str(data.get("access_key", "")).strip()
        if not config.msr.access_key or config.msr.access_key == "change-me":
            return jsonify({"error": "MSR access_key 尚未在配置文件中设置"}), 500
        if not access_key:
            return jsonify({"error": "请输入 access_key"}), 400
        if access_key != config.msr.access_key:
            return jsonify({"error": "access_key 验证失败"}), 403
        return jsonify({"success": True})

    @app.route("/api/msr/query", methods=["POST"])
    def query_msr_image():
        started_at = time.perf_counter()
        data = request.get_json()
        if not data:
            return jsonify({"error": "请提供 JSON 数据"}), 400

        qq_id = str(data.get("qq_id", "")).strip()
        region = str(data.get("region", "jp")).lower().strip()
        game_id = str(data.get("game_id", "")).strip()
        access_key = str(data.get("access_key", "")).strip()
        config = get_upload_config()
        repo = get_msr_repo(config)

        if not qq_id:
            return jsonify({"error": "请输入 QQ 号"}), 400
        if not qq_id.isdigit():
            return jsonify({"error": "QQ 号格式不正确"}), 400
        if region not in VALID_REGIONS:
            return jsonify({"error": f"不支持的区服: {region}"}), 400
        if not game_id:
            return jsonify({"error": "请选择要查询的账号"}), 400
        if not game_id.isdigit():
            return jsonify({"error": "游戏 ID 格式不正确"}), 400
        if not config.msr.access_key or config.msr.access_key == "change-me":
            return jsonify({"error": "MSR access_key 尚未在配置文件中设置"}), 500
        if access_key != config.msr.access_key:
            return jsonify({"error": "access_key 验证失败"}), 403
        if not ensure_bound_account(qq_id, region, game_id):
            return jsonify({"error": "该账号未绑定到当前 QQ 号"}), 403

        print(f"[msr] start region={region} qq={qq_id} game_id={game_id}")
        try:
            image_bytes = render_msr_image(
                repo=repo,
                config=config,
                region=region,
                qq_id=qq_id,
                game_id=game_id,
            )
        except MsrQueryError as exc:
            elapsed = time.perf_counter() - started_at
            print(f"[msr] failed region={region} game_id={game_id} elapsed={elapsed:.2f}s error={exc}")
            return jsonify({"error": str(exc)}), 400
        except Exception as exc:
            elapsed = time.perf_counter() - started_at
            print(f"[msr] failed region={region} game_id={game_id} elapsed={elapsed:.2f}s error={exc}")
            return jsonify({"error": f"MSR 渲染失败: {exc}"}), 500

        filename = f"msr_{region}_{game_id}.png"
        elapsed = time.perf_counter() - started_at
        print(f"[msr] done region={region} game_id={game_id} elapsed={elapsed:.2f}s bytes={len(image_bytes)}")
        return send_file(
            io.BytesIO(image_bytes),
            mimetype="image/png",
            as_attachment=False,
            download_name=filename,
            max_age=0,
        )

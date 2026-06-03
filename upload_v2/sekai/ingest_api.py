from __future__ import annotations

import io
import json
import re
import shutil
import time
from pathlib import Path
from threading import Lock

import requests
from flask import Blueprint, current_app, jsonify, request, send_file

from core.runtime import BASE_DIR, REGION_API_HOSTS, REGION_NAMES, VALID_DATA_TYPES, VALID_REGIONS, get_msr_repo, get_upload_config
from sekai.services.bindings import ensure_bound_account, get_bind_info, load_profile_db, mask_game_id
from sekai.services.crypto import DECRYPT_AVAILABLE, convert_to_serializable, decrypt_binary_data
from sekai.services.data import UploadProcessingError, normalize_upload_payload, process_and_save_data, save_json_payload
from sekai.services.msr_service import MsrQueryError, render_msr_image
from sekai.services.suite import extract_suite_user_id

bp = Blueprint("sekai_ingest_api", __name__, url_prefix="/api")

SEKAI_DIR = Path(__file__).resolve().parent
IOS_UPLOAD_DIR = SEKAI_DIR / "tmp" / "ios_uploads"
IOS_UPLOAD_DIR.mkdir(parents=True, exist_ok=True)
_IOS_UPLOAD_LOCK = Lock()


def _sanitize_upload_id(upload_id: str) -> str | None:
    normalized = re.sub(r"[^0-9A-Za-z_-]", "", upload_id or "")
    if not normalized:
        return None
    return normalized[:80]


def _build_ios_session_dir(upload_id: str) -> Path:
    return IOS_UPLOAD_DIR / upload_id


def _save_ios_chunk(session_dir: Path, chunk_index: int, chunk_data: bytes) -> None:
    session_dir.mkdir(parents=True, exist_ok=True)
    chunk_path = session_dir / f"{chunk_index:08d}.part"
    chunk_path.write_bytes(chunk_data)


def _has_all_ios_chunks(session_dir: Path, total_chunks: int) -> bool:
    for index in range(total_chunks):
        if not (session_dir / f"{index:08d}.part").exists():
            return False
    return True


def _assemble_ios_chunks(session_dir: Path, total_chunks: int) -> bytes:
    parts = []
    for index in range(total_chunks):
        parts.append((session_dir / f"{index:08d}.part").read_bytes())
    return b"".join(parts)


def _cleanup_ios_session(session_dir: Path) -> None:
    shutil.rmtree(session_dir, ignore_errors=True)


def _extract_uid_from_original_url(original_url: str, data_type: str) -> str | None:
    if data_type == "suite":
        match = re.search(r"/api/suite/user/(\d+)(?:$|\?)", original_url)
    else:
        match = re.search(r"/api/user/(\d+)/mysekai(?:$|\?)", original_url)
    return match.group(1) if match else None


@bp.route("/query_binding", methods=["POST"])
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


@bp.route("/msr/validate_access_key", methods=["POST"])
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


@bp.route("/msr/query", methods=["POST"])
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
    return send_file(
        io.BytesIO(image_bytes),
        mimetype="image/png",
        as_attachment=False,
        download_name=filename,
        max_age=0,
    )


@bp.route("/ios/upload", methods=["POST"])
def ios_upload():
    region = request.args.get("region", "").lower()
    data_type = request.args.get("data_type", "").lower()
    if region not in VALID_REGIONS:
        return jsonify({"error": f"Unsupported region: {region}"}), 400
    if data_type not in VALID_DATA_TYPES:
        return jsonify({"error": f"Unsupported data type: {data_type}"}), 400
    if not DECRYPT_AVAILABLE:
        return jsonify({"error": "Server decrypt dependencies are unavailable"}), 500

    upload_id = _sanitize_upload_id(request.headers.get("X-Upload-Id", ""))
    original_url = request.headers.get("X-Original-Url", "")
    if not upload_id:
        return jsonify({"error": "Missing X-Upload-Id"}), 400
    if not original_url:
        return jsonify({"error": "Missing X-Original-Url"}), 400

    try:
        chunk_index = int(request.headers.get("X-Chunk-Index", "-1"))
        total_chunks = int(request.headers.get("X-Total-Chunks", "0"))
    except ValueError:
        return jsonify({"error": "Invalid chunk headers"}), 400

    if chunk_index < 0 or total_chunks <= 0 or chunk_index >= total_chunks:
        return jsonify({"error": "Chunk index out of range"}), 400

    chunk_data = request.get_data(cache=False)
    if chunk_data is None:
        return jsonify({"error": "Missing request body"}), 400

    uid = _extract_uid_from_original_url(original_url, data_type)
    if not uid:
        return jsonify({"error": "Unable to extract user id from original url"}), 400

    session_dir = _build_ios_session_dir(upload_id)
    with _IOS_UPLOAD_LOCK:
        _save_ios_chunk(session_dir, chunk_index, chunk_data)
        if not _has_all_ios_chunks(session_dir, total_chunks):
            return jsonify(
                {
                    "success": True,
                    "complete": False,
                    "upload_id": upload_id,
                    "chunk_index": chunk_index,
                    "total_chunks": total_chunks,
                }
            )
        data_bytes = _assemble_ios_chunks(session_dir, total_chunks)
        _cleanup_ios_session(session_dir)

    try:
        process_and_save_data(region, uid, data_bytes, data_type)
    except Exception as exc:
        return jsonify({"error": str(exc)}), 500
    return jsonify(
        {
            "success": True,
            "complete": True,
            "upload_id": upload_id,
            "user_id": uid,
            "region": region,
            "data_type": data_type,
        }
    )


@bp.route("/<region>/user/<uid>/upload/<data_type>", methods=["GET", "POST", "PUT"])
def proxy_upload(region, uid, data_type="mysekai"):
    region = region.lower()
    data_type = data_type.lower()
    if region not in REGION_API_HOSTS:
        return jsonify({"error": f"Unsupported region: {region}"}), 400
    if data_type not in VALID_DATA_TYPES:
        return jsonify({"error": f"Unsupported data type: {data_type}"}), 400

    content_type = request.headers.get("Content-Type", "")
    if "application/json" in content_type:
        try:
            data = request.get_json()
            if not data:
                return jsonify({"error": "无效的 JSON 数据"}), 400

            if "upload_time" not in data:
                data["upload_time"] = int(time.time() * 1000)
            if "source" not in data:
                data["source"] = "direct_upload"
            if "local_source" not in data:
                data["local_source"] = "direct_upload"
            if "updatedResources" in data:
                if "userMysekaiGamedata" not in data["updatedResources"]:
                    data["updatedResources"]["userMysekaiGamedata"] = {}
                data["updatedResources"]["userMysekaiGamedata"]["userId"] = int(uid)

            if data_type == "suite":
                data = normalize_upload_payload(region, data, data_type, str(uid), data["source"], data["local_source"])
            save_path = save_json_payload(region, data_type, str(uid), data)
            print(f"直接上传成功: {region} user {uid} ({data_type}) -> {save_path}")
            return jsonify({"success": True, "message": f"{region} user {uid} data saved", "file_path": str(save_path)})
        except Exception as exc:
            print(f"处理直接上传数据时出错: {exc}")
            return jsonify({"error": str(exc)}), 500

    target_host = REGION_API_HOSTS[region]
    if data_type == "suite":
        real_api_path = f"/api/suite/user/{uid}"
    else:
        real_api_path = f"/api/user/{uid}/mysekai"

    target_url = f"https://{target_host}{real_api_path}"
    excluded_headers = {"Host", "Content-Length", "Cookie", "Origin", "Referer"}
    headers = {k: v for k, v in request.headers if k not in excluded_headers}
    headers["Host"] = target_host

    try:
        resp = requests.request(
            method=request.method,
            url=target_url,
            headers=headers,
            data=request.get_data(),
            params=request.args,
            timeout=10,
            allow_redirects=False,
        )
        if resp.status_code == 200 and resp.content:
            try:
                process_and_save_data(region, uid, resp.content, data_type)
            except UploadProcessingError as exc:
                print(f"代理数据保存失败: {exc}")
                return jsonify({"error": str(exc)}), 500

        response = current_app.make_response(resp.content)
        response.status_code = resp.status_code
        for key, value in resp.headers.items():
            if key not in ["Transfer-Encoding", "Content-Encoding", "Content-Length"]:
                response.headers[key] = value
        return response
    except Exception as exc:
        print(f"代理请求失败: {exc}")
        return jsonify({"error": "Proxy error"}), 502


@bp.route("/upload/<data_type>", methods=["POST"])
def upload(data_type):
    data_type = data_type.lower()
    if data_type not in VALID_DATA_TYPES:
        return jsonify({"error": f"不支持的数据类型: {data_type}"}), 400

    if "file" not in request.files:
        return jsonify({"error": "没有上传文件"}), 400

    file = request.files["file"]
    if file.filename == "":
        return jsonify({"error": "没有选择文件"}), 400

    region = request.form.get("region", "jp").lower()
    if region not in VALID_REGIONS:
        return jsonify({"error": f"不支持的区服: {region}"}), 400

    if data_type != "suite":
        game_id = request.form.get("game_id", "").strip()
        if not game_id:
            return jsonify({"error": "请选择要上传数据的游戏账号"}), 400
        if not game_id.isdigit():
            return jsonify({"error": "游戏 ID 格式不正确"}), 400
    else:
        game_id = None

    try:
        content = file.read()
        data = None
        is_binary = False

        try:
            text_content = content.decode("utf-8")
            data = json.loads(text_content)
        except (UnicodeDecodeError, json.JSONDecodeError):
            is_binary = True

        if is_binary:
            if not DECRYPT_AVAILABLE:
                return jsonify({"error": "服务器未安装解密依赖，无法处理二进制文件。请安装 sssekai 和 msgpack"}), 500
            try:
                data = decrypt_binary_data(content, region)
                data = convert_to_serializable(data)
            except Exception as exc:
                return jsonify({"error": f"二进制文件解密失败: {exc}"}), 400

        if data_type == "suite":
            game_id = extract_suite_user_id(data)
            if not game_id:
                return jsonify(
                    {
                        "error": "上传失败：无法从文件中提取用户ID。请确保上传的是完整的 Suite 抓包数据文件（包含 userGamedata 字段）"
                    }
                ), 400

            db = load_profile_db()
            region_binds = db.get("bind_list", {}).get(region, {})
            is_bound = False
            for qq_id, bound_ids in region_binds.items():
                if isinstance(bound_ids, str):
                    bound_ids = [bound_ids]
                if game_id in [str(bid) for bid in bound_ids]:
                    is_bound = True
                    break
            if not is_bound:
                return jsonify(
                    {
                        "error": f"上传失败：游戏ID {mask_game_id(game_id)} 尚未在 Bot 中绑定。请先在bot中使用绑定指令绑定你的游戏账号"
                    }
                ), 400

        data = normalize_upload_payload(
            region=region,
            data=data,
            data_type=data_type,
            game_id=str(game_id),
            source="local_upload",
            local_source="web_upload",
        )
        save_path = save_json_payload(region, data_type, str(game_id), data)
        print(f"表单上传成功: {region} user {game_id} ({data_type}) -> {save_path}")

        return jsonify(
            {
                "success": True,
                "user_id": game_id,
                "display_id": mask_game_id(game_id),
                "region": region,
                "region_name": REGION_NAMES.get(region, region),
                "data_type": data_type,
                "file_path": str(save_path),
                "upload_time": data["upload_time"],
                "was_binary": is_binary,
            }
        )
    except json.JSONDecodeError as exc:
        return jsonify({"error": f"JSON 解析错误: {exc}"}), 400
    except Exception as exc:
        return jsonify({"error": f"处理文件时出错: {exc}"}), 500

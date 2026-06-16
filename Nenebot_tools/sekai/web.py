from __future__ import annotations

from pathlib import Path

from flask import Blueprint, jsonify, redirect, request, send_from_directory

from core.runtime import CATCHER_DIR, DOWNLOAD_FILES, PROFILE_DB_PATH, VALID_DATA_TYPES, VALID_REGIONS, get_upload_config
from sekai.services.help_texts import (
    build_all_help_ios,
    build_ios_upload_script,
    build_mysekai_help_android,
)
from sekai.services.crypto import DECRYPT_AVAILABLE

bp = Blueprint("sekai_web", __name__, url_prefix="/upload")
SEKAI_DIR = Path(__file__).resolve().parent
PAGES_DIR = SEKAI_DIR / "pages"
STATIC_DIR = SEKAI_DIR / "static"


def _send_frontend_file(filename: str):
    return send_from_directory(str(PAGES_DIR), filename)


def _send_static_file(filename: str):
    return send_from_directory(str(STATIC_DIR), filename)


def _get_request_host() -> str:
    return request.headers.get("X-Forwarded-Host") or request.headers.get("Host", "")


@bp.route("/", strict_slashes=False)
def index():
    return redirect("/upload/help")


@bp.route("/suite")
def suite_page():
    return _send_frontend_file("index.html")


@bp.route("/help")
def help_page():
    return _send_frontend_file("help.html")


@bp.route("/mysekai")
def mysekai_page():
    return _send_frontend_file("mysekai.html")


@bp.route("/msr")
def msr_page():
    return _send_frontend_file("msr.html")


@bp.route("/styles.css")
def styles():
    return _send_static_file("styles.css")


@bp.route("/script.js")
def script():
    return _send_static_file("script.js")


@bp.route("/msr.js")
def msr_script():
    return _send_static_file("msr.js")


@bp.route("/help.js")
def help_script():
    return _send_static_file("help.js")


@bp.route("/help/ios", methods=["GET"])
def all_help_ios():
    host = _get_request_host()
    return build_all_help_ios(host), 200, {"Content-Type": "text/plain; charset=utf-8"}


@bp.route("/help/android", methods=["GET"])
def help_android():
    host = _get_request_host()
    return build_mysekai_help_android(host), 200, {"Content-Type": "text/plain; charset=utf-8"}


@bp.route("/public/scripts/upload.js", methods=["GET"])
def ios_upload_script():
    host = _get_request_host()
    region = request.args.get("region", "cn").lower()
    data_type = request.args.get("data_type", "mysekai").lower()
    if region not in VALID_REGIONS:
        return jsonify({"error": f"Unsupported region: {region}"}), 400
    if data_type not in VALID_DATA_TYPES:
        return jsonify({"error": f"Unsupported data type: {data_type}"}), 400
    return build_ios_upload_script(host, region, data_type), 200, {
        "Content-Type": "application/javascript; charset=utf-8"
    }


@bp.route("/scripts/kill", methods=["GET"])
def killcatcher():
    scripts_dir = CATCHER_DIR / "scripts"
    return send_from_directory(str(scripts_dir), "kill-catcher.sh", mimetype="text/x-shellscript")


@bp.route("/download/<filename>")
def download_file(filename):
    if filename not in DOWNLOAD_FILES:
        return jsonify({"error": "文件不存在"}), 404
    return send_from_directory(DOWNLOAD_FILES[filename], filename)


@bp.route("/status", methods=["GET"])
def status():
    config = get_upload_config()
    return jsonify(
        {
            "status": "ok",
            "version": "2.0.0",
            "supported_regions": VALID_REGIONS,
            "decrypt_available": DECRYPT_AVAILABLE,
            "profile_db_available": PROFILE_DB_PATH.exists(),
            "msr_available": True,
            "msr_access_key_configured": bool(
                config.msr.access_key and config.msr.access_key != "change-me"
            ),
        }
    )

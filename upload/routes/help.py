from __future__ import annotations

from flask import jsonify, redirect, request, send_from_directory

from core.help_texts import (
    build_all_help_ios,
    build_ios_upload_script,
    build_mysekai_help_android,
    build_suite_help_ios,
)
from core.runtime import CATCHER_DIR, DOWNLOAD_FILES, VALID_DATA_TYPES, VALID_REGIONS


def register_help_routes(app):
    def get_request_host() -> str:
        return request.headers.get("X-Forwarded-Host") or request.headers.get("Host", "")

    @app.route("/help/ios", methods=["GET"])
    @app.route("/upload/help/ios", methods=["GET"])
    def all_help_ios():
        host = get_request_host()
        return build_all_help_ios(host), 200, {"Content-Type": "text/plain; charset=utf-8"}

    @app.route("/help/android", methods=["GET"])
    @app.route("/upload/help/android", methods=["GET"])
    def help_android():
        host = get_request_host()
        return build_mysekai_help_android(host), 200, {"Content-Type": "text/plain; charset=utf-8"}

    @app.route("/public/scripts/upload.js", methods=["GET"])
    @app.route("/upload/public/scripts/upload.js", methods=["GET"])
    def ios_upload_script():
        host = get_request_host()
        region = request.args.get("region", "cn").lower()
        data_type = request.args.get("data_type", "mysekai").lower()
        if region not in VALID_REGIONS:
            return jsonify({"error": f"Unsupported region: {region}"}), 400
        if data_type not in VALID_DATA_TYPES:
            return jsonify({"error": f"Unsupported data type: {data_type}"}), 400
        return build_ios_upload_script(host, region, data_type), 200, {
            "Content-Type": "application/javascript; charset=utf-8"
        }

    @app.route("/scripts/kill", methods=["GET"])
    @app.route("/upload/scripts/kill", methods=["GET"])
    def killcatcher():
        scripts_dir = CATCHER_DIR / "scripts"
        return send_from_directory(str(scripts_dir), "kill-catcher.sh", mimetype="text/x-shellscript")

    @app.route("/download/<filename>")
    @app.route("/upload/download/<filename>")
    def download_file(filename):
        if filename not in DOWNLOAD_FILES:
            return jsonify({"error": "文件不存在"}), 404
        return send_from_directory(DOWNLOAD_FILES[filename], filename)

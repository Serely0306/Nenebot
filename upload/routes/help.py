from __future__ import annotations

from flask import jsonify, request, send_from_directory

from core.help_texts import build_mysekai_help_android, build_mysekai_help_ios, build_suite_help_ios
from core.runtime import CATCHER_DIR, DOWNLOAD_FILES


def register_help_routes(app):
    @app.route("/suite/help/ios", methods=["GET"])
    def suite_help_ios():
        host = request.headers.get("Host")
        return build_suite_help_ios(host), 200, {"Content-Type": "text/plain; charset=utf-8"}

    @app.route("/mysekai/help/ios", methods=["GET"])
    def mysekai_help_ios():
        host = request.headers.get("Host")
        return build_mysekai_help_ios(host), 200, {"Content-Type": "text/plain; charset=utf-8"}

    @app.route("/mysekai/help/android", methods=["GET"])
    def mysekai_help_android():
        host = request.headers.get("Host")
        return build_mysekai_help_android(host), 200, {"Content-Type": "text/plain; charset=utf-8"}

    @app.route("/scripts/kill", methods=["GET"])
    def killcatcher():
        scripts_dir = CATCHER_DIR / "scripts"
        return send_from_directory(str(scripts_dir), "kill-catcher.sh", mimetype="text/x-shellscript")

    @app.route("/download/<filename>")
    def download_file(filename):
        if filename not in DOWNLOAD_FILES:
            return jsonify({"error": "文件不存在"}), 404
        return send_from_directory(DOWNLOAD_FILES[filename], filename)

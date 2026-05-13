from __future__ import annotations

from flask import redirect, send_from_directory

from core.runtime import BASE_DIR


def register_static_routes(app):
    def send_frontend_file(filename: str):
        return send_from_directory(str(BASE_DIR), filename)

    @app.route("/")
    def index():
        return redirect("help")

    @app.route("/suite")
    def suite_page():
        return send_frontend_file("index.html")

    @app.route("/help")
    def help_page():
        return send_frontend_file("help.html")

    @app.route("/mysekai")
    def mysekai_page():
        return send_frontend_file("mysekai.html")

    @app.route("/msr")
    def msr_page():
        return send_frontend_file("msr.html")

    @app.route("/styles.css")
    def styles():
        return send_frontend_file("styles.css")

    @app.route("/script.js")
    def script():
        return send_frontend_file("script.js")

    @app.route("/msr.js")
    def msr_script():
        return send_frontend_file("msr.js")

    @app.route("/help.js")
    def help_script():
        return send_frontend_file("help.js")

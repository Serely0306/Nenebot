from __future__ import annotations

from flask import redirect, send_from_directory

from core.runtime import BASE_DIR


def register_static_routes(app):
    def send_frontend_file(filename: str):
        return send_from_directory(str(BASE_DIR), filename)

    @app.route("/")
    @app.route("/upload")
    @app.route("/upload/")
    def index():
        return redirect("/upload/help")

    @app.route("/suite")
    @app.route("/upload/suite")
    def suite_page():
        return send_frontend_file("index.html")

    @app.route("/help")
    @app.route("/upload/help")
    def help_page():
        return send_frontend_file("help.html")

    @app.route("/mysekai")
    @app.route("/upload/mysekai")
    def mysekai_page():
        return send_frontend_file("mysekai.html")

    @app.route("/msr")
    @app.route("/upload/msr")
    def msr_page():
        return send_frontend_file("msr.html")

    @app.route("/styles.css")
    @app.route("/upload/styles.css")
    def styles():
        return send_frontend_file("styles.css")

    @app.route("/script.js")
    @app.route("/upload/script.js")
    def script():
        return send_frontend_file("script.js")

    @app.route("/msr.js")
    @app.route("/upload/msr.js")
    def msr_script():
        return send_frontend_file("msr.js")

    @app.route("/help.js")
    @app.route("/upload/help.js")
    def help_script():
        return send_frontend_file("help.js")

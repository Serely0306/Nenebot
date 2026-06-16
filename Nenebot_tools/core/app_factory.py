from __future__ import annotations

from flask import Flask
from flask_cors import CORS
from werkzeug.middleware.proxy_fix import ProxyFix

from apply import register_apply_routes
from sekai.bot_api import bp as sekai_bot_api_bp
from sekai.ingest_api import bp as sekai_ingest_api_bp
from sekai.web import bp as sekai_web_bp
from site_root import site_bp


def create_app() -> Flask:
    app = Flask(__name__, static_folder=None)
    app.wsgi_app = ProxyFix(app.wsgi_app, x_for=1, x_proto=1)
    app.config["JSON_AS_ASCII"] = False
    try:
        app.json.ensure_ascii = False
    except AttributeError:
        pass
    CORS(app)
    app.config["MAX_CONTENT_LENGTH"] = 50 * 1024 * 1024

    app.register_blueprint(site_bp)
    app.register_blueprint(sekai_web_bp)
    app.register_blueprint(sekai_ingest_api_bp)
    app.register_blueprint(sekai_bot_api_bp)
    register_apply_routes(app)
    return app

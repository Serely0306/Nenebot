from __future__ import annotations

from flask import Flask
from flask_cors import CORS

from core.runtime import BASE_DIR
from apply import register_apply_routes
from routes.help import register_help_routes
from routes.msr import register_msr_routes
from routes.proxy import register_proxy_routes
from routes.query import register_query_routes
from routes.static import register_static_routes


def create_app() -> Flask:
    app = Flask(__name__, static_folder=str(BASE_DIR), static_url_path="")
    app.config["JSON_AS_ASCII"] = False
    try:
        app.json.ensure_ascii = False
    except AttributeError:
        pass
    CORS(app)
    app.config["MAX_CONTENT_LENGTH"] = 50 * 1024 * 1024

    register_static_routes(app)
    register_query_routes(app)
    register_msr_routes(app)
    register_proxy_routes(app)
    register_help_routes(app)
    register_apply_routes(app)
    return app

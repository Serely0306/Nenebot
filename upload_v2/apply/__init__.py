from __future__ import annotations

from apply.routes import bp


def register_apply_routes(app):
    app.register_blueprint(bp)

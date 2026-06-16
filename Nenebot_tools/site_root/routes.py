from __future__ import annotations

from flask import Blueprint, redirect

bp = Blueprint("site_root", __name__)


@bp.route("/")
def index():
    return redirect("/upload/help")

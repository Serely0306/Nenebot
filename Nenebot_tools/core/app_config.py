from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


BASE_DIR = Path(__file__).resolve().parent.parent
CONFIG_PATH = BASE_DIR / "config.yaml"


@dataclass
class MsrConfig:
    access_key: str = "change-me"
    asset_request_timeout_seconds: int = 15
    render_show_harvested: bool = False


@dataclass
class CryptoKeysetConfig:
    key_hex: str = ""
    iv_hex: str = ""


@dataclass
class CryptoConfig:
    keysets: dict[str, CryptoKeysetConfig]


@dataclass
class UploadConfig:
    msr: MsrConfig
    crypto: CryptoConfig


def _deep_get(data: dict[str, Any], path: str, default: Any) -> Any:
    current: Any = data
    for key in path.split("."):
        if not isinstance(current, dict) or key not in current:
            return default
        current = current[key]
    return current


def load_upload_config() -> UploadConfig:
    raw: dict[str, Any] = {}
    if CONFIG_PATH.exists():
        with CONFIG_PATH.open("r", encoding="utf-8") as fh:
            raw = yaml.safe_load(fh) or {}

    raw_keysets = _deep_get(raw, "crypto.keysets", {})
    keysets = {
        str(region): CryptoKeysetConfig(
            key_hex=str(values.get("key_hex", "")),
            iv_hex=str(values.get("iv_hex", "")),
        )
        for region, values in raw_keysets.items()
        if isinstance(values, dict)
    }

    return UploadConfig(
        msr=MsrConfig(
            access_key=str(_deep_get(raw, "msr.access_key", "change-me")),
            asset_request_timeout_seconds=int(
                _deep_get(raw, "msr.asset_request_timeout_seconds", 15)
            ),
            render_show_harvested=bool(
                _deep_get(raw, "msr.render_show_harvested", False)
            ),
        ),
        crypto=CryptoConfig(keysets=keysets),
    )

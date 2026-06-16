from __future__ import annotations

from pathlib import Path
from threading import Lock

from core.app_config import UploadConfig, load_upload_config
from sekai.services.sekai_assets import SekaiAssetRepository


BASE_DIR = Path(__file__).resolve().parent.parent

VALID_DATA_TYPES = ["suite", "mysekai"]
VALID_REGIONS = ["jp", "cn", "tw", "kr", "en"]
REGION_TO_KEYSET = {
    "jp": "jp",
    "cn": "cn",
    "tw": "tw",
    "kr": "kr",
    "en": "en",
}
REGION_NAMES = {
    "jp": "日服",
    "cn": "国服",
    "tw": "台服",
    "kr": "韩服",
    "en": "国际服",
}

LUNABOT_BASE = BASE_DIR.parent / "lunabot"
LUNABOT_DATA_BASE = LUNABOT_BASE / "data" / "sekai" / "user_data"
PROFILE_DB_PATH = LUNABOT_BASE / "data" / "sekai" / "profile" / "db.json"

REGION_API_HOSTS = {
    "jp": "production-game-api.sekai.colorfulpalette.org",
    "cn": "mkcn-prod-public-60001-2.dailygn.com",
    "tw": "prod-api.sekai-pl.com",
    "kr": "prod-api.sekai-m.com",
    "en": "production-game-api.sekai.colorfulstage.com",
}

CATCHER_DIR = BASE_DIR.parent / "catcher"
DOWNLOAD_FILES = {
    "Catcher-android-arm64": str(CATCHER_DIR),
    "config-android.yaml": str(CATCHER_DIR),
    "catcher.sh": str(CATCHER_DIR / "scripts"),
    "kill-catcher.sh": str(CATCHER_DIR / "scripts"),
    "uninstall-catcher.sh": str(CATCHER_DIR / "scripts"),
}

_MSR_REPO_LOCK = Lock()
_MSR_REPO_BY_TIMEOUT: dict[int, SekaiAssetRepository] = {}


def get_upload_config() -> UploadConfig:
    return load_upload_config()


def get_msr_repo(config: UploadConfig | None = None) -> SekaiAssetRepository:
    current_config = config or get_upload_config()
    timeout = int(current_config.msr.asset_request_timeout_seconds)
    with _MSR_REPO_LOCK:
        repo = _MSR_REPO_BY_TIMEOUT.get(timeout)
        if repo is None:
            repo = SekaiAssetRepository(timeout)
            _MSR_REPO_BY_TIMEOUT[timeout] = repo
        return repo


UPLOAD_CONFIG = get_upload_config()
MSR_REPO = get_msr_repo(UPLOAD_CONFIG)

from __future__ import annotations

import io
import json
import re
from pathlib import Path
from typing import Any
from urllib.parse import urlsplit, urlunsplit

import requests
import yaml
from PIL import Image


REPO_ROOT = Path(__file__).resolve().parents[3]
LUNABOT_BASE = REPO_ROOT / "lunabot"
SEKAI_ASSET_CONFIG_PATH = LUNABOT_BASE / "config" / "sekai" / "asset.yaml"
SITE_MAP_INFO_PATH = LUNABOT_BASE / "config" / "sekai" / "mysekai_site_map_image_info.yaml"
PHENOMENA_COLORS_PATH = (
    LUNABOT_BASE / "config" / "sekai" / "mysekai_phenomena_colors.yaml"
)
SEKAI_CONFIG_PATH = LUNABOT_BASE / "config" / "sekai" / "sekai.yaml"
MASTERDATA_DIR = LUNABOT_BASE / "data" / "sekai" / "assets" / "masterdata"
RIP_DIR = LUNABOT_BASE / "data" / "sekai" / "assets" / "rip"
STATIC_IMAGE_DIR = LUNABOT_BASE / "data" / "sekai" / "assets" / "static_images"
STATIC_IMAGE_DUMP_DIR = (
    LUNABOT_BASE / "data" / "sekai" / "assets" / "static_images" / "mysekai" / "image"
)
STATIC_IMAGE_ALIAS_PATHS = {
    "mysekai/site/grassland.png": [
        "mysekai/site/sitemap/texture/img_harvest_site_5.png",
        "mysekai/site/sitemap/texture/img_map_site_5.png",
    ],
    "mysekai/site/beach.png": [
        "mysekai/site/sitemap/texture/img_harvest_site_6.png",
        "mysekai/site/sitemap/texture/img_map_site_6.png",
    ],
    "mysekai/site/flowergarden.png": [
        "mysekai/site/sitemap/texture/img_harvest_site_7.png",
        "mysekai/site/sitemap/texture/img_map_site_7.png",
    ],
    "mysekai/site/memorialplace.png": [
        "mysekai/site/sitemap/texture/img_harvest_site_8.png",
        "mysekai/site/sitemap/texture/img_map_site_8.png",
    ],
    "mysekai/mark.png": [
        "mysekai/site/sitemap/texture/tex_sitemap_pt.png",
    ],
    "mysekai/light.png": [
        "mysekai/site/sitemap/texture/tex_sitemap_flare_back.png",
    ],
}
LOCAL_ONLY_RIP_PREFIXES = (
    "mysekai/harvest_fixture_icon/",
)

ONDEMAND_PREFIXES = ["event", "gacha", "music/long", "mysekai", "virtual_live"]
STARTAPP_PREFIXES = [
    "bonds_honor",
    "honor",
    "thumbnail",
    "character",
    "music",
    "rank_live",
    "stamp",
    "home/banner",
    "player_frame",
    "areaitem",
]
_ASSETS_PREFIXES = (
    "jp-assets/",
    "cn-assets/",
    "en-assets/",
    "tw-assets/",
    "kr-assets/",
)


def _load_yaml(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as fh:
        return yaml.safe_load(fh) or {}


def _open_image_from_bytes(data: bytes) -> Image.Image:
    image = Image.open(io.BytesIO(data))
    image.load()
    return image.convert("RGBA")


def sekai_best_url_map(url: str) -> str:
    mapped = url.replace("_rip", "")
    if "music_score" in mapped and not mapped.endswith(".txt"):
        mapped += ".txt"
    return mapped


def haruki_url_map(url: str) -> str:
    split = urlsplit(url)
    path = split.path.lstrip("/")
    region_prefix = ""
    for prefix in _ASSETS_PREFIXES:
        if path.startswith(prefix):
            region_prefix = prefix
            path = path[len(prefix) :]
            break

    if path.startswith("assets/"):
        path = path[len("assets/") :]

    mapped = path.replace("_rip", "").replace(".asset", ".json")
    if "music_score" in mapped and not mapped.endswith(".txt"):
        mapped += ".txt"

    if not re.match(r"^(startapp|ondemand)/", mapped):
        if any(mapped.startswith(prefix) for prefix in ONDEMAND_PREFIXES):
            category = "ondemand"
        elif any(mapped.startswith(prefix) for prefix in STARTAPP_PREFIXES):
            category = "startapp"
        else:
            category = "ondemand"
        mapped = f"{category}/{mapped}"

    return urlunsplit(
        (split.scheme, split.netloc, "/" + region_prefix + mapped, split.query, split.fragment)
    )


def pjsekai_moe_url_map(url: str) -> str:
    return url.replace("_rip", "")


def unipjsk_url_map(url: str) -> str:
    idx = url.find("assets.unipjsk.com/")
    if idx == -1:
        return url
    idx += len("assets.unipjsk.com/")
    prefix, path = url[:idx], url[idx:]
    path = path.replace("_rip", "").replace(".asset", ".json")
    if any(path.startswith(item) for item in ONDEMAND_PREFIXES):
        category = "ondemand"
    elif any(path.startswith(item) for item in STARTAPP_PREFIXES):
        category = "startapp"
    else:
        category = "ondemand"
    return f"{prefix}{category}/{path}"


URL_MAP_METHODS = {
    "sekai.best": sekai_best_url_map,
    "haruki": haruki_url_map,
    "pjsekai.moe": pjsekai_moe_url_map,
    "unipjsk": unipjsk_url_map,
}


class SekaiAssetRepository:
    def __init__(self, request_timeout: int = 15):
        self.request_timeout = request_timeout
        self.asset_config = _load_yaml(SEKAI_ASSET_CONFIG_PATH)
        self.site_map_info = _load_yaml(SITE_MAP_INFO_PATH)
        self.phenomena_colors = _load_yaml(PHENOMENA_COLORS_PATH)
        self.sekai_config = _load_yaml(SEKAI_CONFIG_PATH)
        self._masterdata_cache: dict[tuple[str, str], Any] = {}
        self._masterdata_failure_cache: dict[tuple[str, str], str] = {}
        self._rip_bytes_cache: dict[tuple[str, str], bytes] = {}
        self._rip_image_cache: dict[tuple[str, str], Image.Image] = {}
        self._rip_failure_cache: dict[tuple[str, str], str] = {}
        self._static_bytes_cache: dict[tuple[str, str], bytes] = {}
        self._static_image_cache: dict[tuple[str, str], Image.Image] = {}
        self._static_failure_cache: dict[tuple[str, str], str] = {}

    def _get_rip_sources(self, region: str) -> list[dict[str, Any]]:
        return list(self.asset_config.get(region, {}).get("rip", {}).get("sources", []))

    def _get_master_sources(self, region: str) -> list[dict[str, Any]]:
        return list(
            self.asset_config.get(region, {}).get("masterdata", {}).get("sources", [])
        )

    def _download_bytes(self, url: str) -> bytes:
        response = requests.get(url, timeout=self.request_timeout)
        response.raise_for_status()
        return response.content

    def _download_json(self, url: str) -> Any:
        response = requests.get(url, timeout=self.request_timeout)
        response.raise_for_status()
        return response.json()

    def get_masterdata(self, region: str, name: str) -> Any:
        cache_key = (region, name)
        if cache_key in self._masterdata_cache:
            return self._masterdata_cache[cache_key]
        if cache_key in self._masterdata_failure_cache:
            raise RuntimeError(self._masterdata_failure_cache[cache_key])

        path = MASTERDATA_DIR / region / f"{name}.json"
        if not path.exists():
            path.parent.mkdir(parents=True, exist_ok=True)
            last_error: Exception | None = None
            for source in self._get_master_sources(region):
                base_url = str(source.get("base_url", "")).strip()
                if not base_url:
                    continue
                if not base_url.endswith("/"):
                    base_url += "/"
                url = f"{base_url}{name}.json"
                try:
                    data = self._download_json(url)
                    with path.open("w", encoding="utf-8") as fh:
                        json.dump(data, fh, ensure_ascii=False, indent=2)
                    break
                except Exception as exc:
                    last_error = exc
            if not path.exists():
                message = f"failed to load masterdata {region}/{name}: {last_error}"
                self._masterdata_failure_cache[cache_key] = message
                raise RuntimeError(message)

        with path.open("r", encoding="utf-8") as fh:
            data = json.load(fh)
        self._masterdata_cache[cache_key] = data
        return data

    def _map_rip_url(self, source: dict[str, Any], path: str) -> str:
        base_url = str(source.get("base_url", "")).strip()
        if not base_url.endswith("/"):
            base_url += "/"
        mapper = URL_MAP_METHODS.get(source.get("name", ""))
        url = base_url + path
        return mapper(url) if mapper else url

    def _iter_static_candidates(self, path: str) -> list[Path]:
        logical = Path(path)
        candidates: list[Path] = [
            STATIC_IMAGE_DIR / logical,
            STATIC_IMAGE_DUMP_DIR / logical,
        ]

        if path.startswith("mysekai/harvest_fixture_icon/"):
            stem = logical.stem
            candidates.extend(
                [
                    STATIC_IMAGE_DUMP_DIR / "mysekai" / "thumbnail" / "fixture" / f"{stem}_1.png",
                    STATIC_IMAGE_DUMP_DIR / "mysekai" / "thumbnail" / "fixture" / f"{stem}.png",
                ]
            )
        return candidates

    def _iter_static_aliases(self, path: str) -> list[str]:
        return list(STATIC_IMAGE_ALIAS_PATHS.get(path, []))

    def get_static_bytes(
        self,
        path: str,
        region: str | None = None,
        *,
        allow_remote_fallback: bool = False,
    ) -> bytes:
        cache_key = (region or "", path)
        if cache_key in self._static_bytes_cache:
            return self._static_bytes_cache[cache_key]
        if cache_key in self._static_failure_cache:
            raise RuntimeError(self._static_failure_cache[cache_key])

        for candidate in self._iter_static_candidates(path):
            if candidate.exists():
                data = candidate.read_bytes()
                self._static_bytes_cache[cache_key] = data
                return data

        for alias_path in self._iter_static_aliases(path):
            for candidate in self._iter_static_candidates(alias_path):
                if candidate.exists():
                    data = candidate.read_bytes()
                    self._static_bytes_cache[cache_key] = data
                    return data

            if allow_remote_fallback and region:
                for fallback_region in (region, "jp"):
                    try:
                        data = self.get_rip_bytes(
                            fallback_region,
                            alias_path,
                            allow_remote_for_local_only=True,
                        )
                        self._static_bytes_cache[cache_key] = data
                        return data
                    except Exception:
                        continue

        if allow_remote_fallback and region:
            try:
                data = self.get_rip_bytes(region, path, allow_remote_for_local_only=True)
                self._static_bytes_cache[cache_key] = data
                return data
            except Exception as exc:
                message = f"failed to load static asset {path}: {exc}"
                self._static_failure_cache[cache_key] = message
                raise RuntimeError(message) from exc

        message = f"static asset not found: {path}"
        self._static_failure_cache[cache_key] = message
        raise RuntimeError(message)

    def get_static_image(
        self,
        path: str,
        region: str | None = None,
        *,
        allow_remote_fallback: bool = False,
    ) -> Image.Image:
        cache_key = (region or "", path)
        if cache_key in self._static_image_cache:
            return self._static_image_cache[cache_key].copy()

        image = _open_image_from_bytes(
            self.get_static_bytes(
                path,
                region,
                allow_remote_fallback=allow_remote_fallback,
            )
        )
        self._static_image_cache[cache_key] = image
        return image.copy()

    def get_rip_bytes(
        self,
        region: str,
        path: str,
        *,
        allow_remote_for_local_only: bool = False,
    ) -> bytes:
        cache_key = (region, path)
        if cache_key in self._rip_bytes_cache:
            return self._rip_bytes_cache[cache_key]
        if cache_key in self._rip_failure_cache:
            raise RuntimeError(self._rip_failure_cache[cache_key])

        cache_path = RIP_DIR / region / Path(path)
        if cache_path.exists():
            data = cache_path.read_bytes()
            self._rip_bytes_cache[cache_key] = data
            return data

        for candidate in self._iter_static_candidates(path):
            if not candidate.exists():
                continue
            data = candidate.read_bytes()
            self._rip_bytes_cache[cache_key] = data
            return data

        if (
            not allow_remote_for_local_only
            and any(path.startswith(prefix) for prefix in LOCAL_ONLY_RIP_PREFIXES)
        ):
            message = f"local-only rip asset not found: {region}/{path}"
            self._rip_failure_cache[cache_key] = message
            raise RuntimeError(message)

        cache_path.parent.mkdir(parents=True, exist_ok=True)
        last_error: Exception | None = None
        for source in self._get_rip_sources(region):
            prefixes = source.get("prefixes")
            if prefixes and not any(path.startswith(prefix) for prefix in prefixes):
                continue
            url = self._map_rip_url(source, path)
            try:
                data = self._download_bytes(url)
                cache_path.write_bytes(data)
                self._rip_bytes_cache[cache_key] = data
                print(f"[msr-assets] downloaded {region}/{path} from {source.get('name', 'unknown')}")
                return data
            except Exception as exc:
                last_error = exc
        message = f"failed to load rip asset {region}/{path}: {last_error}"
        self._rip_failure_cache[cache_key] = message
        print(f"[msr-assets] missing {region}/{path}: {last_error}")
        raise RuntimeError(message)

    def get_rip_image(self, region: str, path: str) -> Image.Image:
        cache_key = (region, path)
        if cache_key in self._rip_image_cache:
            return self._rip_image_cache[cache_key].copy()

        image = _open_image_from_bytes(self.get_rip_bytes(region, path))
        self._rip_image_cache[cache_key] = image
        return image.copy()

    def get_site_map_info(self, site_id: int) -> dict[str, Any]:
        return dict(self.site_map_info.get(site_id) or self.site_map_info.get(str(site_id)) or {})

    def get_phenomena_color_info(self, phenomena_id: int) -> dict[str, tuple[int, int, int]]:
        item = self.phenomena_colors.get(phenomena_id) or self.phenomena_colors.get(str(phenomena_id))
        if not item:
            return {
                "ground": (255, 255, 255),
                "sky1": (130, 190, 255),
                "sky2": (80, 120, 220),
            }
        brightness = float(self.sekai_config.get("mysekai", {}).get("map_brightness", 1.0))
        ground = self._hex_to_rgb(item["ground"])
        sky1 = self._hex_to_rgb(item["sky1"])
        sky2 = self._hex_to_rgb(item["sky2"])
        if brightness > 1.0:
            ground = self._lerp_color(ground, (255, 255, 255), brightness - 1.0)
        elif brightness < 1.0:
            ground = self._lerp_color(ground, (0, 0, 0), 1.0 - brightness)
        return {"ground": ground, "sky1": sky1, "sky2": sky2}

    def get_rare_res_keys(self) -> dict[str, list[str]]:
        return self.sekai_config.get("mysekai", {}).get("rare_res_keys", {})

    @staticmethod
    def _lerp_color(c1: tuple[int, int, int], c2: tuple[int, int, int], t: float) -> tuple[int, int, int]:
        t = max(0.0, min(1.0, t))
        return tuple(int(a + (b - a) * t) for a, b in zip(c1, c2))

    @staticmethod
    def _hex_to_rgb(color: str) -> tuple[int, int, int]:
        color = color.lstrip("#")
        return tuple(int(color[i : i + 2], 16) for i in (0, 2, 4))


def resize_keep_ratio(image: Image.Image, size: float | tuple[int | None, int | None], mode: str = "scale") -> Image.Image:
    if mode == "scale" and not isinstance(size, tuple):
        scale = float(size)
        target = (
            max(1, int(image.width * scale)),
            max(1, int(image.height * scale)),
        )
        return image.resize(target, Image.LANCZOS)

    width, height = size  # type: ignore[misc]
    if width is None and height is None:
        return image.copy()
    if width is None:
        scale = height / image.height
        width = max(1, int(image.width * scale))
    elif height is None:
        scale = width / image.width
        height = max(1, int(image.height * scale))
    return image.resize((int(width), int(height)), Image.LANCZOS)


def multiply_image_by_color(image: Image.Image, color: tuple[int, int, int]) -> Image.Image:
    tinted = Image.new("RGBA", image.size, (*color, 255))
    return Image.blend(image.convert("RGBA"), tinted, 0.35)


def build_vertical_gradient(size: tuple[int, int], top: tuple[int, int, int], bottom: tuple[int, int, int]) -> Image.Image:
    width, height = size
    gradient = Image.new("RGBA", size)
    pixels = gradient.load()
    for y in range(height):
        factor = y / max(1, height - 1)
        color = tuple(int(top[i] + (bottom[i] - top[i]) * factor) for i in range(3))
        for x in range(width):
            pixels[x, y] = (*color, 255)
    return gradient

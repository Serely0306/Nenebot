from __future__ import annotations

import io
import math
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

from PIL import Image, ImageDraw, ImageFilter, ImageFont

from core.app_config import UploadConfig
from core.runtime import LUNABOT_BASE, REGION_NAMES
from sekai.services.bindings import get_bind_info, mask_game_id
from sekai.services.sekai_assets import (
    SekaiAssetRepository,
    build_vertical_gradient,
    multiply_image_by_color,
    resize_keep_ratio,
)


LUNABOT_DATA_BASE = LUNABOT_BASE / "data" / "sekai" / "user_data"
SITE_ID_ORDER = (5, 7, 6, 8)
SITE_NAMES = {
    5: "原野",
    7: "花圃",
    6: "海滩",
    8: "纪念地",
}
REGION_UTC_OFFSET = {
    "jp": 9,
    "cn": 8,
    "tw": 8,
    "kr": 9,
    "en": -7,
}
SITE_BG_DOWNSAMPLE = 0.5
RESOURCE_ICON_ALPHA = 0.8
_WARNED_MESSAGES: set[str] = set()


class MsrQueryError(Exception):
    pass


@dataclass
class ResDrawCall:
    image: Image.Image
    quantity: int
    small_icon: bool
    size: int
    x: int
    z: int
    draw_order: int
    rarity: int
    outline: tuple[tuple[int, int, int, int], int] | None = None
    light_size: int | None = None


def _warn_once(message: str) -> None:
    if message in _WARNED_MESSAGES:
        return
    _WARNED_MESSAGES.add(message)
    print(message)


def _load_json(path: Path) -> dict[str, Any]:
    import json

    with path.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def _hex_font(size: int) -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    try:
        return ImageFont.truetype(str(LUNABOT_BASE / "simkai.ttf"), size=size)
    except Exception:
        return ImageFont.load_default()


def _apply_alpha(image: Image.Image, factor: float) -> Image.Image:
    alpha = image.getchannel("A").point(lambda value: int(max(0, min(255, value * factor))))
    tinted = image.copy()
    tinted.putalpha(alpha)
    return tinted


def _region_hour_to_local(region: str, hour: int) -> int:
    return hour + REGION_UTC_OFFSET["cn"] - REGION_UTC_OFFSET.get(region, 8)


def _get_refresh_hours(region: str) -> tuple[int, int]:
    return (_region_hour_to_local(region, 5), _region_hour_to_local(region, 17))


def _get_mysekai_cfg(repo: SekaiAssetRepository, key: str, default: Any) -> Any:
    current: Any = repo.sekai_config.get("mysekai", {})
    for part in key.split("."):
        if not isinstance(current, dict) or part not in current:
            return default
        current = current[part]
    return current


def _get_res_rarity(repo: SekaiAssetRepository, key: str) -> int:
    try:
        prefix, raw_id = key.rsplit("_", 1)
        res_id = int(raw_id)
    except ValueError:
        return 0

    for rarity, values in repo.get_rare_res_keys().items():
        for item in values:
            if "~" in item:
                item_prefix, raw_range = item.rsplit("_", 1)
                min_id, max_id = [int(x) for x in raw_range.split("~", 1)]
                if prefix == item_prefix and min_id <= res_id <= max_id:
                    return int(rarity)
            elif item == key:
                return int(rarity)
    return 0


def _make_marker(size: int, fill: tuple[int, int, int, int]) -> Image.Image:
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    draw.ellipse((2, 2, size - 2, size - 2), fill=fill, outline=(255, 255, 255, 220), width=2)
    return image


def _make_light(size: int) -> Image.Image:
    image = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(image)
    draw.ellipse((6, 6, size - 6, size - 6), fill=(255, 220, 120, 120))
    return image.filter(ImageFilter.GaussianBlur(radius=max(2, size // 8)))


def _make_placeholder_icon(label: str, size: int, fill: tuple[int, int, int]) -> Image.Image:
    image = Image.new("RGBA", (size, size), (*fill, 255))
    draw = ImageDraw.Draw(image)
    draw.rounded_rectangle(
        (0, 0, size - 1, size - 1),
        radius=max(4, size // 6),
        outline=(255, 255, 255, 180),
        width=2,
    )
    text = label[:3]
    font = _hex_font(max(12, size // 4))
    bbox = draw.textbbox((0, 0), text, font=font)
    tw = bbox[2] - bbox[0]
    th = bbox[3] - bbox[1]
    draw.text(((size - tw) / 2, (size - th) / 2 - 2), text, fill="white", font=font)
    return image


def _make_fixture_fallback(fixture_type: str, rarity: str, size: int) -> Image.Image:
    colors = {
        ("wood", "rarity_1"): (84, 132, 74),
        ("wood", "rarity_2"): (166, 120, 53),
        ("mineral", "rarity_1"): (100, 116, 128),
        ("mineral", "rarity_2"): (92, 123, 190),
        ("toolbox", "rarity_1"): (190, 160, 64),
        ("plant", "rarity_1"): (214, 127, 178),
        ("plant", "rarity_2"): (171, 90, 183),
        ("other", "rarity_1"): (118, 118, 118),
        ("driftage", "rarity_1"): (113, 150, 170),
        ("tone", "rarity_1"): (155, 94, 179),
    }
    fill = colors.get((fixture_type, rarity), (110, 110, 110))
    short = {
        "wood": "木",
        "mineral": "矿",
        "toolbox": "箱",
        "plant": "花",
        "other": "杂",
        "driftage": "漂",
        "tone": "音",
        "birthday_plant": "生",
    }.get(fixture_type, "点")
    return _make_placeholder_icon(short, size, fill)


def _safe_get_static_image(
    repo: SekaiAssetRepository,
    path: str,
    *,
    region: str | None = None,
    allow_remote_fallback: bool = False,
) -> Image.Image | None:
    try:
        return repo.get_static_image(
            path,
            region,
            allow_remote_fallback=allow_remote_fallback,
        )
    except Exception:
        return None


def _get_fixture_icon(
    repo: SekaiAssetRepository,
    region: str,
    harvest_fixture: dict[str, Any],
    point_drop_group: dict[str, dict[str, Any]],
    masterdata: dict[str, dict[int, dict[str, Any]]],
    scale: float,
) -> tuple[Image.Image | None, tuple[int, int]]:
    fixture_type = str(harvest_fixture.get("mysekaiSiteHarvestFixtureType", "other"))
    rarity = str(harvest_fixture.get("mysekaiSiteHarvestFixtureRarityType", "rarity_1"))
    asset_name = str(harvest_fixture.get("assetbundleName", "unknown"))

    if fixture_type == "birthday_plant":
        image: Image.Image | None = None
        chara_id: int | None = None
        for item in point_drop_group.values():
            if item["type"] == "material" and 174 <= int(item["id"]) <= 199:
                chara_id = int(item["id"]) - 173
                break
        if chara_id:
            chara = masterdata["gameCharacters"].get(chara_id)
            if chara:
                given_name = str(chara.get("givenNameEnglish", "")).lower()
                current_year = datetime.now().year
                for year in (current_year, current_year - 1):
                    try:
                        image = repo.get_rip_image(
                            region,
                            f"mysekai/birthday/{given_name}_{year}/icon_refresh.png",
                        )
                        break
                    except Exception:
                        continue
        if image is None:
            _warn_once(f"[msr-assets] missing birthday harvest point icon: {asset_name}")
            image = Image.new("RGBA", (32, 32), (0, 0, 0, 0))

        point_img_size = 50 * scale
        xoffset = point_img_size * 0.15
        zoffset = 0
        image = resize_keep_ratio(image, (int(point_img_size), None))
        offset = (
            int(-point_img_size * 0.5 + xoffset),
            int(-point_img_size * 0.5 + zoffset),
        )
        return image, offset

    point_img_size = 160 * scale
    xoffset = 0
    zoffset = -point_img_size * 0.3
    image = _safe_get_static_image(
        repo,
        f"mysekai/harvest_fixture_icon/{rarity}/{asset_name}.png",
    )
    if image is None:
        _warn_once(f"[msr-assets] missing harvest point icon: {asset_name}")
        return None, (0, 0)
    image = resize_keep_ratio(image, (int(point_img_size), None))
    offset = (
        int(-point_img_size * 0.5 + xoffset),
        int(-point_img_size * 0.5 + zoffset),
    )
    return image, offset


def _get_res_icon(
    repo: SekaiAssetRepository,
    region: str,
    key: str,
    masterdata: dict[str, dict[int, dict[str, Any]]],
) -> Image.Image:
    try:
        res_id = int(key.split("_")[-1])
    except ValueError:
        return _make_placeholder_icon("?", 48, (110, 110, 110))

    try:
        if key.startswith("mysekai_material"):
            item = masterdata["mysekaiMaterials"][res_id]
            return repo.get_rip_image(
                region,
                f"mysekai/thumbnail/material/{item['iconAssetbundleName']}.png",
            )
        if key.startswith("material"):
            return repo.get_rip_image(region, f"thumbnail/material_rip/material{res_id}.png")
        if key.startswith("mysekai_item"):
            item = masterdata["mysekaiItems"][res_id]
            return repo.get_rip_image(
                region,
                f"mysekai/thumbnail/item/{item['iconAssetbundleName']}.png",
            )
        if key.startswith("mysekai_fixture"):
            item = masterdata["mysekaiFixtures"][res_id]
            try:
                return repo.get_rip_image(
                    region,
                    f"mysekai/thumbnail/fixture/{item['assetbundleName']}_{res_id}_1.png",
                )
            except Exception:
                return repo.get_rip_image(
                    region,
                    f"mysekai/thumbnail/fixture/{item['assetbundleName']}_1.png",
                )
        if key.startswith("mysekai_music_record"):
            record = masterdata["mysekaiMusicRecords"][res_id]
            music = masterdata["musics"][int(record["externalId"])]
            return repo.get_rip_image(
                region,
                f"music/jacket/{music['assetbundleName']}_rip/{music['assetbundleName']}.png",
            )
    except Exception:
        pass

    return _make_placeholder_icon(str(res_id), 48, (110, 110, 110))


def _build_masterdata_indexes(
    repo: SekaiAssetRepository,
    region: str,
) -> dict[str, dict[int, dict[str, Any]]]:
    def indexed(name: str) -> dict[int, dict[str, Any]]:
        return {int(item["id"]): item for item in repo.get_masterdata(region, name)}

    return {
        "mysekaiMaterials": indexed("mysekaiMaterials"),
        "mysekaiItems": indexed("mysekaiItems"),
        "mysekaiFixtures": indexed("mysekaiFixtures"),
        "mysekaiMusicRecords": indexed("mysekaiMusicRecords"),
        "mysekaiSiteHarvestFixtures": indexed("mysekaiSiteHarvestFixtures"),
        "musics": indexed("musics"),
        "gameCharacters": indexed("gameCharacters"),
    }


def _compose_site_map(
    repo: SekaiAssetRepository,
    region: str,
    harvest_map: dict[str, Any],
    show_harvested: bool,
    phenomena_color_info: dict[str, tuple[int, int, int]],
    masterdata: dict[str, dict[int, dict[str, Any]]],
) -> Image.Image:
    site_id = int(harvest_map["mysekaiSiteId"])
    site_image_info = repo.get_site_map_info(site_id)
    if not site_image_info:
        raise MsrQueryError(f"缺少 site {site_id} 的地图配置")

    scale = float(_get_mysekai_cfg(repo, "map_image_scale", 0.75))
    site_image = _safe_get_static_image(
        repo,
        str(site_image_info["image"]),
        region=region,
        allow_remote_fallback=True,
    )
    if site_image is None:
        _warn_once(f"[msr-assets] missing site image: {site_image_info['image']}")
        site_image = Image.new("RGBA", (1280, 1080), (*phenomena_color_info["ground"], 255))

    draw_w, draw_h = int(site_image.width * scale), int(site_image.height * scale)
    mid_x, mid_z = draw_w / 2, draw_h / 2
    grid_size = float(site_image_info["grid_size"]) * scale
    offset_x = float(site_image_info["offset_x"]) * scale
    offset_z = float(site_image_info["offset_z"]) * scale
    dir_x = int(site_image_info["dir_x"])
    dir_z = int(site_image_info["dir_z"])
    rev_xz = bool(site_image_info["rev_xz"])

    crop_bbox = site_image_info.get("crop_bbox")
    if crop_bbox:
        crop_x, crop_y, crop_w, crop_h = [int(item) for item in crop_bbox]
        site_image = site_image.crop((crop_x, crop_y, crop_x + crop_w, crop_y + crop_h))
        draw_w = int(crop_w * scale)
        draw_h = int(crop_h * scale)
        offset_x -= crop_x * scale
        offset_z -= crop_y * scale

    site_image = resize_keep_ratio(site_image, SITE_BG_DOWNSAMPLE, "scale")
    site_image = multiply_image_by_color(site_image, phenomena_color_info["ground"])
    site_image = resize_keep_ratio(site_image, (draw_w, draw_h))

    def game_pos_to_draw_pos(x: float, z: float) -> tuple[int, int]:
        if rev_xz:
            x, z = z, x
        x = x * grid_size * dir_x
        z = z * grid_size * dir_z
        x += mid_x + offset_x
        z += mid_z + offset_z
        x = max(0, min(x, draw_w))
        z = max(0, min(z, draw_h))
        return int(x), int(z)

    all_res: dict[str, dict[str, dict[str, Any]]] = defaultdict(dict)
    for item in harvest_map.get("userMysekaiSiteHarvestResourceDrops", []):
        res_key = f"{item['resourceType']}_{item['resourceId']}"
        status = item.get("mysekaiSiteHarvestResourceDropStatus")
        if not show_harvested and status != "before_drop":
            continue

        x, z = game_pos_to_draw_pos(item["positionX"], item["positionZ"])
        pkey = f"{x}_{z}"
        if res_key not in all_res[pkey]:
            all_res[pkey][res_key] = {
                "id": int(item["resourceId"]),
                "type": item["resourceType"],
                "x": x,
                "z": z,
                "quantity": int(item["quantity"]),
                "image": _get_res_icon(repo, region, res_key, masterdata),
                "small_icon": False,
                "del": False,
            }
        else:
            all_res[pkey][res_key]["quantity"] += int(item["quantity"])

    def is_birthday_drop(res: dict[str, Any]) -> bool:
        return res["type"] == "material" and 174 <= int(res["id"]) <= 199

    for pkey, group in all_res.items():
        is_birthday_sapling = False
        is_cotton_flower = False
        has_material_drop = False
        for res_key, item in group.items():
            if res_key in {"mysekai_material_1", "mysekai_material_6"} and item["quantity"] == 6:
                group[res_key]["del"] = True
            if res_key in {"mysekai_material_21", "mysekai_material_22"}:
                is_cotton_flower = True
            if res_key.startswith("mysekai_material"):
                has_material_drop = True
            if is_birthday_drop(item) and item["quantity"] > 16:
                is_birthday_sapling = True
        for res_key, item in group.items():
            if not res_key.startswith("mysekai_material") and has_material_drop:
                group[res_key]["small_icon"] = True
            if is_cotton_flower and res_key not in {"mysekai_material_21", "mysekai_material_22"}:
                group[res_key]["small_icon"] = True
            if is_birthday_sapling:
                group[res_key]["small_icon"] = not is_birthday_drop(item)
            elif is_birthday_drop(item):
                group[res_key]["del"] = True

    harvest_points: list[dict[str, Any]] = []
    harvest_point_fid_pkeys: dict[int, str] = {}
    for item in harvest_map.get("userMysekaiSiteHarvestFixtures", []):
        fixture_id = int(item["mysekaiSiteHarvestFixtureId"])
        status = item.get("userMysekaiSiteHarvestFixtureStatus")
        if not show_harvested and status != "spawned":
            continue
        x, z = game_pos_to_draw_pos(item["positionX"], item["positionZ"])
        harvest_point_fid_pkeys[fixture_id] = f"{x}_{z}"
        harvest_points.append({"id": fixture_id, "x": x, "z": z})
    harvest_points.sort(key=lambda item: (item["z"], item["x"]))

    canvas = site_image.copy()
    draw = ImageDraw.Draw(canvas)

    for point in harvest_points:
        harvest_fixture = masterdata["mysekaiSiteHarvestFixtures"].get(point["id"])
        if not harvest_fixture:
            continue
        point_pkey = harvest_point_fid_pkeys.get(point["id"], "")
        point_drop_group = all_res.get(point_pkey, {})
        icon, offset = _get_fixture_icon(
            repo,
            region,
            harvest_fixture,
            point_drop_group,
            masterdata,
            scale,
        )
        if icon is None:
            continue
        canvas.alpha_composite(icon, (point["x"] + offset[0], point["z"] + offset[1]))

    spawn_img = _safe_get_static_image(
        repo,
        "mysekai/mark.png",
        region=region,
        allow_remote_fallback=True,
    )
    if spawn_img is None:
        _warn_once("[msr-assets] missing spawn marker: mysekai/mark.png")
        spawn_img = _make_marker(max(14, int(20 * scale)), (255, 84, 94, 230))
    spawn_size = int(20 * scale)
    spawn_img = resize_keep_ratio(spawn_img, (spawn_size, spawn_size))
    spawn_x, spawn_z = game_pos_to_draw_pos(0, 0)
    canvas.alpha_composite(spawn_img, (spawn_x - spawn_size // 2, spawn_z - spawn_size // 2))

    res_draw_calls: list[ResDrawCall] = []
    large_light_factor = float(_get_mysekai_cfg(repo, "rare_res_light.large_size", 7.0))
    small_light_factor = float(_get_mysekai_cfg(repo, "rare_res_light.small_size", 5.0))

    for group in all_res.values():
        pres = sorted(
            [item for item in group.values() if not item["del"]],
            key=lambda item: (-item["quantity"], item["id"]),
        )
        large_total = sum(1 for item in pres if not item["small_icon"])
        small_idx = 0
        large_idx = 0
        icon_zoffset = -160 * scale * 0.2

        for item in pres:
            if not item["image"]:
                continue

            res_key = f"{item['type']}_{item['id']}"
            rarity = _get_res_rarity(repo, res_key)
            large_size = 35 * scale
            small_size = 17 * scale
            if item["type"] == "mysekai_material" and item["id"] == 24:
                large_size *= 1.5
            if item["type"] == "mysekai_music_record":
                large_size *= 1.5

            if item["small_icon"]:
                size = small_size
                x = int(item["x"] + 0.5 * large_size * large_total - 0.6 * small_size)
                z = int(item["z"] - 0.45 * large_size + 1.0 * small_size * small_idx + icon_zoffset)
                small_idx += 1
            else:
                size = large_size
                x = int(item["x"] - 0.5 * large_size * large_total + large_size * large_idx)
                z = int(item["z"] - 0.5 * large_size + icon_zoffset)
                large_idx += 1

            if z <= 0:
                z += int(0.5 * large_size)

            if item["small_icon"]:
                draw_order = item["z"] * 100 + item["x"] + 1_000_000
            elif rarity == 2:
                draw_order = item["z"] * 100 + item["x"] + 100_000
            else:
                draw_order = item["z"] * 100 + item["x"]

            outline: tuple[tuple[int, int, int, int], int] | None = None
            if rarity == 2:
                outline = ((255, 50, 50, 150), 2)
            elif item["small_icon"]:
                outline = ((50, 50, 255, 100), 1)

            light_size: int | None = None
            if rarity == 2 and not res_key.startswith("material"):
                if item["small_icon"]:
                    light_size = int(45 * scale * small_light_factor)
                else:
                    light_size = int(45 * scale * large_light_factor)

            res_draw_calls.append(
                ResDrawCall(
                    image=item["image"],
                    quantity=item["quantity"],
                    small_icon=item["small_icon"],
                    size=int(size),
                    x=x,
                    z=z,
                    draw_order=draw_order,
                    rarity=rarity,
                    outline=outline,
                    light_size=light_size,
                )
            )

    res_draw_calls.sort(key=lambda item: item.draw_order)

    light_img = _safe_get_static_image(
        repo,
        "mysekai/light.png",
        region=region,
        allow_remote_fallback=True,
    )
    light_strength = (
        phenomena_color_info["ground"][0]
        + phenomena_color_info["ground"][1]
        + phenomena_color_info["ground"][2]
    ) / (3 * 255)
    light_effect = float(_get_mysekai_cfg(repo, "rare_res_light.map_brightness_effect", 0.5))
    light_strength = 1.0 * (1.0 - light_effect) + light_strength * light_effect

    for call in res_draw_calls:
        if not call.light_size:
            continue
        glow = light_img
        if glow is None:
            glow = _make_light(call.light_size)
        glow = resize_keep_ratio(glow, (call.light_size, call.light_size))
        glow = _apply_alpha(glow, light_strength)
        gx = int(call.x + call.size / 2 - call.light_size / 2)
        gz = int(call.z + call.size / 2 - call.light_size / 2)
        canvas.alpha_composite(glow, (gx, gz))

    qty_font = _hex_font(max(10, int(11 * scale)))
    qty_font_big = _hex_font(max(12, int(13 * scale)))
    for call in res_draw_calls:
        if call.outline:
            stroke, stroke_w = call.outline
            draw.rounded_rectangle(
                (
                    call.x - stroke_w,
                    call.z - stroke_w,
                    call.x + call.size + stroke_w,
                    call.z + call.size + stroke_w,
                ),
                radius=max(4, int(6 * scale)),
                outline=stroke,
                width=stroke_w,
            )
        image = resize_keep_ratio(call.image, (call.size, call.size))
        canvas.alpha_composite(_apply_alpha(image, RESOURCE_ICON_ALPHA), (call.x, call.z))

    for call in res_draw_calls:
        if call.small_icon:
            continue
        color = (50, 50, 50)
        font = qty_font
        if call.quantity == 2:
            color = (200, 20, 0)
            font = qty_font_big
        elif call.quantity > 2:
            color = (200, 20, 200)
            font = qty_font_big

        x_offset, z_offset = -1, -1
        if call.quantity >= 10:
            x_offset = 1
            z_offset = int(call.size - 13 * scale) - 3
        draw.text((call.x + x_offset, call.z + z_offset), str(call.quantity), fill=color, font=font)

    draw.rounded_rectangle(
        (2, 2, canvas.width - 3, canvas.height - 3),
        radius=18,
        outline=(255, 255, 255, 110),
        width=2,
    )
    title_font = _hex_font(22)
    title = SITE_NAMES.get(site_id, f"Site {site_id}")
    text_box = Image.new("RGBA", (200, 44), (0, 0, 0, 0))
    tdraw = ImageDraw.Draw(text_box)
    tdraw.rounded_rectangle((0, 0, 199, 43), radius=14, fill=(20, 20, 28, 140))
    tdraw.text((14, 10), title, fill="white", font=title_font)
    canvas.alpha_composite(text_box, (16, 16))
    return canvas


def render_msr_image(
    repo: SekaiAssetRepository,
    config: UploadConfig,
    region: str,
    qq_id: str,
    game_id: str,
) -> bytes:
    file_path = LUNABOT_DATA_BASE / region / "mysekai" / f"{game_id}.json"
    if not file_path.exists():
        raise MsrQueryError(
            f"找不到 {REGION_NAMES.get(region, region)} 账号 {mask_game_id(game_id)} 的 MySekai 数据"
        )

    mysekai_info = _load_json(file_path)
    updated_resources = mysekai_info.get("updatedResources", {})
    harvest_maps = updated_resources.get("userMysekaiHarvestMaps")
    if not harvest_maps:
        raise MsrQueryError("该账号的 MySekai 数据不完整，缺少资源地图数据")

    upload_time_raw = int(mysekai_info.get("upload_time", 0))
    if upload_time_raw <= 0:
        raise MsrQueryError("该账号的 MySekai 数据缺少 upload_time")
    upload_time = datetime.fromtimestamp(upload_time_raw / 1000)

    schedule = mysekai_info.get("mysekaiPhenomenaSchedules") or []
    h1, h2 = _get_refresh_hours(region)
    current_hour = upload_time.hour
    phenom_idx = 1 if current_hour < h1 or current_hour >= h2 else 0
    phenom_ids = [item.get("mysekaiPhenomenaId") for item in schedule]
    current_phenom_id = phenom_ids[phenom_idx] if len(phenom_ids) > phenom_idx else 1
    phenomena_color_info = repo.get_phenomena_color_info(int(current_phenom_id))

    masterdata = _build_masterdata_indexes(repo, region)
    site_map_lookup = {int(item["mysekaiSiteId"]): item for item in harvest_maps}
    site_images: list[Image.Image] = []
    for site_id in SITE_ID_ORDER:
        site_map = site_map_lookup.get(site_id)
        if not site_map:
            continue
        site_images.append(
            _compose_site_map(
                repo,
                region,
                site_map,
                config.msr.render_show_harvested,
                phenomena_color_info,
                masterdata,
            )
        )

    if not site_images:
        raise MsrQueryError("没有可绘制的 MySekai 地图")

    cols = 2
    gap = 20
    padding = 28
    header_height = 92
    card_width = max(image.width for image in site_images)
    card_height = max(image.height for image in site_images)
    rows = math.ceil(len(site_images) / cols)
    canvas_size = (
        padding * 2 + cols * card_width + (cols - 1) * gap,
        padding * 2 + header_height + rows * card_height + (rows - 1) * gap,
    )
    canvas = build_vertical_gradient(
        canvas_size,
        phenomena_color_info["sky1"],
        phenomena_color_info["sky2"],
    )
    draw = ImageDraw.Draw(canvas)

    title_font = _hex_font(32)
    meta_font = _hex_font(18)
    draw.text(
        (padding, padding),
        f"MySekai MSR 地图 - {REGION_NAMES.get(region, region)}",
        fill="white",
        font=title_font,
        stroke_width=2,
        stroke_fill=(0, 0, 0),
    )
    bind_info = get_bind_info(qq_id, region)
    qq_text = f"QQ: {qq_id}  账号: {mask_game_id(game_id)}"
    if bind_info.get("main_id") == str(game_id):
        qq_text += "  主账号"
    draw.text((padding, padding + 42), qq_text, fill=(245, 245, 245), font=meta_font)
    draw.text(
        (padding, padding + 66),
        f"上传时间: {upload_time.strftime('%Y-%m-%d %H:%M:%S')}",
        fill=(235, 235, 235),
        font=meta_font,
    )

    start_y = padding + header_height
    for index, image in enumerate(site_images):
        row = index // cols
        col = index % cols
        x = padding + col * (card_width + gap)
        y = start_y + row * (card_height + gap)
        canvas.alpha_composite(image, (x, y))

    output = io.BytesIO()
    canvas.save(output, format="PNG")
    return output.getvalue()

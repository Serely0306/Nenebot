from __future__ import annotations

import json
import time
from pathlib import Path

from core.runtime import LUNABOT_DATA_BASE
from services.crypto import DECRYPT_AVAILABLE, convert_to_serializable, decrypt_binary_data
from services.suite import merge_suite_incremental_fields, process_suite_compact


def load_and_filter_json(file_path: Path, filter_keys: list[str]):
    with file_path.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    if filter_keys:
        filtered = {k: v for k, v in data.items() if k in filter_keys}
        for meta_key in ["upload_time", "source", "local_source"]:
            if meta_key in data:
                filtered[meta_key] = data[meta_key]
        return filtered
    return data


def build_save_dir(region: str, data_type: str) -> Path:
    save_dir = LUNABOT_DATA_BASE / region / data_type
    save_dir.mkdir(parents=True, exist_ok=True)
    return save_dir


def inject_user_id_if_needed(data: dict, data_type: str, game_id: str) -> dict:
    if data_type == "mysekai" and "updatedResources" in data:
        if "userMysekaiGamedata" not in data["updatedResources"]:
            data["updatedResources"]["userMysekaiGamedata"] = {}
        data["updatedResources"]["userMysekaiGamedata"]["userId"] = int(game_id)
    return data


def save_json_payload(region: str, data_type: str, game_id: str, data: dict) -> Path:
    save_dir = build_save_dir(region, data_type)
    save_path = save_dir / f"{game_id}.json"
    with save_path.open("w", encoding="utf-8") as fh:
        json.dump(data, fh, ensure_ascii=False, indent=2)
    return save_path


def normalize_upload_payload(region: str, data: dict, data_type: str, game_id: str, source: str, local_source: str) -> dict:
    if "upload_time" not in data:
        data["upload_time"] = int(time.time() * 1000)
    data["source"] = source
    data["local_source"] = local_source
    inject_user_id_if_needed(data, data_type, game_id)
    if data_type == "suite":
        data = process_suite_compact(data)
        existing_path = LUNABOT_DATA_BASE / region / data_type / f"{game_id}.json"
        if existing_path.exists():
            with existing_path.open("r", encoding="utf-8") as fh:
                existing_data = process_suite_compact(json.load(fh))
            data = merge_suite_incremental_fields(data, existing_data)
    return data


def process_and_save_data(region, uid, data_bytes, data_type="mysekai"):
    try:
        if not DECRYPT_AVAILABLE:
            print("警告: 解密依赖未安装，无法保存代理抓取的数据")
            return

        try:
            decrypted_data = decrypt_binary_data(data_bytes, region)
            data = convert_to_serializable(decrypted_data)
        except Exception as exc:
            print(f"代理数据解密失败: {exc}")
            return

        data = normalize_upload_payload(
            region=region,
            data=data,
            data_type=data_type,
            game_id=str(uid),
            source="proxy_upload",
            local_source="proxy_upload",
        )
        save_path = save_json_payload(region, data_type, str(uid), data)
        print(f"代理抓包成功: {region} user {uid} ({data_type}) -> {save_path}")
    except Exception as exc:
        print(f"处理代理数据时出错: {exc}")

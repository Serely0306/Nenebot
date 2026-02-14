#!/usr/bin/env python3
"""Import structured seed file into autochat memory."""

from __future__ import annotations

import argparse
import asyncio
import os
import sys
import time
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[3]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
os.chdir(ROOT)

from src.services.autochat.memory import MemorySystem  # noqa: E402
from src.services.autochat.utils import Config, RpcSession  # noqa: E402


config = Config("chat.autochat")


def load_yaml(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}
    if not isinstance(data, dict):
        raise ValueError("seed yaml root must be a mapping/dict")
    return data


def normalize_text_list(items: Any) -> list[str]:
    result: list[str] = []
    if not items:
        return result
    if not isinstance(items, list):
        raise ValueError("expected list")
    for item in items:
        if isinstance(item, str):
            text = item.strip()
        elif isinstance(item, dict):
            text = str(item.get("text", "")).strip()
        else:
            raise ValueError(f"unsupported list item type: {type(item)}")
        if text:
            result.append(text)
    return result


async def query_embeddings(texts: list[str], emb_model: str) -> list[list[float]]:
    rpc = RpcSession(
        config.item("rpc.host"),
        config.item("rpc.port"),
        config.item("rpc.token"),
        config.item("rpc.reconnect_interval"),
    )
    await rpc.connect()
    try:
        return await rpc.call("query_embedding", texts, emb_model, timeout=180)
    finally:
        await rpc.close()


def existing_event_texts(mem: MemorySystem) -> set[str]:
    data = mem.em_collection.get(include=["metadatas"])
    ret: set[str] = set()
    for md in data.get("metadatas", []):
        text = str((md or {}).get("text", "")).strip()
        if text:
            ret.add(text)
    return ret


def existing_self_texts(mem: MemorySystem) -> set[str]:
    return {s.text.strip() for s in mem.sm_get() if s.text and s.text.strip()}


async def main_async(args: argparse.Namespace) -> int:
    seed_path = Path(args.seed)
    if not seed_path.exists():
        print(f"[ERROR] seed file not found: {seed_path}")
        return 1

    seed = load_yaml(seed_path)
    group_id = args.group_id if args.group_id is not None else seed.get("group_id")
    if group_id is None:
        print("[ERROR] missing group_id (use --group-id or group_id in seed yaml)")
        return 1
    group_id = int(group_id)

    mem = MemorySystem("data/chat/autochat", group_id)
    mem_cfg = config.get("chat.mem")
    emb_model = args.emb_model or config.get("chat.llm.emb_model")
    promote_threshold = int(mem_cfg.get("em_long_term_threshold", 10))

    long_term_events = normalize_text_list(seed.get("long_term_events"))
    self_memories = normalize_text_list(seed.get("self_memories"))
    user_memories = seed.get("user_memories", [])
    if user_memories and not isinstance(user_memories, list):
        raise ValueError("user_memories must be a list")

    added_events = 0
    skipped_events = 0
    updated_users = 0
    added_sms = 0

    # 1) Event memories (with embeddings)
    if long_term_events:
        exists = existing_event_texts(mem) if args.skip_existing else set()
        to_add = []
        for text in long_term_events:
            if text in exists:
                skipped_events += 1
                continue
            to_add.append(text)

        if to_add:
            print(f"[INFO] querying embeddings: {len(to_add)} texts, model={emb_model}")
            embs = await query_embeddings(to_add, emb_model)
            for text, emb in zip(to_add, embs):
                em_id = mem.em_add(text=text, embedding=emb, initial_weight=0)
                if args.promote_long_term:
                    mem.em_increase_weight(
                        memory_id=em_id,
                        weight_increase=promote_threshold,
                        threshold=promote_threshold,
                    )
                added_events += 1

    # 2) User memories
    for item in user_memories:
        if not isinstance(item, dict):
            continue
        user_id = item.get("user_id")
        if user_id is None:
            continue
        names = item.get("names", [])
        if names is not None and not isinstance(names, list):
            names = [str(names)]
        profile = item.get("profile")
        events = item.get("recent_events", [])
        if events is not None and not isinstance(events, list):
            events = [str(events)]
        mem.um_update(
            user_id=int(user_id),
            new_names=[str(x).strip() for x in names if str(x).strip()],
            profile_update=str(profile).strip() if profile else None,
            max_events=int(mem_cfg.get("um_max_events", 5)),
            max_names=int(mem_cfg.get("um_max_names", 10)),
        )
        for ev in events:
            ev_text = str(ev).strip()
            if ev_text:
                mem.um_update(
                    user_id=int(user_id),
                    event_update=ev_text,
                    max_events=int(mem_cfg.get("um_max_events", 5)),
                    max_names=int(mem_cfg.get("um_max_names", 10)),
                )
        updated_users += 1

    # 3) Self memories
    if self_memories:
        exists_sm = existing_self_texts(mem) if args.skip_existing else set()
        keep_count = int(mem_cfg.get("sm_keep_count", 10))
        base_id = int(time.time() * 1000)
        for idx, text in enumerate(self_memories):
            if text in exists_sm:
                continue
            mem.sm_add(msg_id=base_id + idx, text=text, keep_count=keep_count)
            added_sms += 1

    print("[OK] seed import completed")
    print(f"[OK] group_id={group_id}")
    print(f"[OK] event_memories added={added_events}, skipped={skipped_events}")
    print(f"[OK] user_memories updated={updated_users}")
    print(f"[OK] self_memories added={added_sms}")
    return 0


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description="Import seed yaml into autochat memory")
    p.add_argument("--seed", required=True, help="seed yaml path")
    p.add_argument("--group-id", type=int, default=None, help="target group id override")
    p.add_argument("--emb-model", default=None, help="override embedding model name")
    p.add_argument("--skip-existing", action="store_true", help="skip duplicate texts")
    p.add_argument("--promote-long-term", action="store_true", default=True, help="promote event memories to long_term")
    p.add_argument("--no-promote-long-term", action="store_false", dest="promote_long_term")
    return p


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return asyncio.run(main_async(args))


if __name__ == "__main__":
    raise SystemExit(main())


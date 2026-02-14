#!/usr/bin/env python3
"""Backup and restore helper for autochat memory data."""

from __future__ import annotations

import argparse
import json
import re
import shutil
from datetime import datetime
from pathlib import Path
from typing import Iterable


DATA_DIR = Path("data/chat/autochat")


def parse_group_id(name: str, prefix: str) -> int | None:
    m = re.fullmatch(rf"{re.escape(prefix)}(\d+)", name)
    return int(m.group(1)) if m else None


def discover_group_ids() -> set[int]:
    ids: set[int] = set()
    if not DATA_DIR.exists():
        return ids
    for p in DATA_DIR.glob("memory_*.json"):
        gid = parse_group_id(p.stem, "memory_")
        if gid is not None:
            ids.add(gid)
    for p in DATA_DIR.glob("memory_chromadb_*"):
        gid = parse_group_id(p.name, "memory_chromadb_")
        if gid is not None:
            ids.add(gid)
    return ids


def copy_entry(src: Path, dst: Path) -> bool:
    if not src.exists():
        return False
    dst.parent.mkdir(parents=True, exist_ok=True)
    if src.is_dir():
        shutil.copytree(src, dst, dirs_exist_ok=True)
    else:
        shutil.copy2(src, dst)
    return True


def iter_group_entries(group_id: int) -> Iterable[tuple[Path, Path]]:
    mem_json = DATA_DIR / f"memory_{group_id}.json"
    mem_chroma = DATA_DIR / f"memory_chromadb_{group_id}"
    yield mem_json, Path("data/chat/autochat") / mem_json.name
    yield mem_chroma, Path("data/chat/autochat") / mem_chroma.name


def backup(groups: list[int], output_dir: Path, include_runtime: bool) -> int:
    output_dir.mkdir(parents=True, exist_ok=True)
    copied = 0
    manifest = {
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "groups": groups,
        "files": [],
    }

    for gid in groups:
        for src, rel in iter_group_entries(gid):
            dst = output_dir / rel
            if copy_entry(src, dst):
                copied += 1
                manifest["files"].append(str(rel))

    if include_runtime:
        for rel in ("data/chat/autochat/db.json", "data/chat/autochat/image_captions.json"):
            src = Path(rel)
            dst = output_dir / rel
            if copy_entry(src, dst):
                copied += 1
                manifest["files"].append(rel)

    (output_dir / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    return copied


def main() -> int:
    parser = argparse.ArgumentParser(description="Backup autochat memory files.")
    parser.add_argument("--group-id", type=int, action="append", dest="group_ids", help="Target group id, repeatable")
    parser.add_argument("--all", action="store_true", help="Backup all discovered groups")
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path("data/chat/autochat/backups") / datetime.now().strftime("%Y%m%d_%H%M%S"),
        help="Backup output directory",
    )
    parser.add_argument("--no-runtime", action="store_true", help="Do not backup db.json/image_captions.json")
    args = parser.parse_args()

    if not DATA_DIR.exists():
        print(f"[ERROR] data dir not found: {DATA_DIR}")
        return 1

    groups: list[int]
    if args.group_ids:
        groups = sorted(set(args.group_ids))
    elif args.all:
        groups = sorted(discover_group_ids())
    else:
        print("[ERROR] specify --group-id ... or --all")
        return 1

    if not groups:
        print("[ERROR] no target groups found")
        return 1

    copied = backup(groups, args.output_dir, include_runtime=not args.no_runtime)
    print(f"[OK] backup created: {args.output_dir}")
    print(f"[OK] groups: {groups}")
    print(f"[OK] entries copied: {copied}")
    print("[TIP] rollback: stop services first, then copy backup files back to project root.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())


#!/usr/bin/env python3
"""Run the SHIP-003 PTY scenarios with isolated roots and deterministic cleanup.

Each scenario is a separate Python process because the harness resolves its
workspace root at import time.  Keeping one root per scenario also prevents a
failed run from making a later scenario pass through stale sessions or logs.
"""

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


HERE = Path(__file__).resolve().parent


def parse_scenarios(value: str) -> list[int]:
    result = []
    for raw in value.split(","):
        raw = raw.strip()
        if not raw:
            continue
        number = int(raw)
        if number < 1 or number > 8:
            raise argparse.ArgumentTypeError("场景编号必须在 1 到 8 之间")
        if number not in result:
            result.append(number)
    if not result:
        raise argparse.ArgumentTypeError("至少指定一个场景")
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description="复跑隔离的 TUI PTY 验收场景")
    parser.add_argument(
        "--scenarios",
        type=parse_scenarios,
        default=list(range(1, 9)),
        help="逗号分隔的场景编号，默认 1,2,3,4,5,6,7,8",
    )
    parser.add_argument(
        "--root",
        type=Path,
        help="保存各场景 runs 的父目录；不指定时运行结束自动清理",
    )
    parser.add_argument(
        "--keep",
        action="store_true",
        help="保留自动创建的临时父目录，便于失败复盘",
    )
    args = parser.parse_args()

    owned_root = args.root is None
    parent = args.root or Path(tempfile.mkdtemp(prefix="basework-ship003-all-"))
    parent.mkdir(parents=True, exist_ok=True)
    env_base = dict(os.environ)
    env_base["PYTHONDONTWRITEBYTECODE"] = "1"

    statuses = []
    try:
        for number in args.scenarios:
            root = parent / f"scenario{number}"
            if root.exists():
                shutil.rmtree(root)
            command = [sys.executable, str(HERE / f"scenario{number}.py")]
            print(f"=== scenario{number} ({root}) ===", flush=True)
            env = dict(env_base)
            env["SHIP003_ROOT"] = str(root)
            completed = subprocess.run(command, cwd=HERE.parent.parent, env=env, check=False)
            statuses.append((number, completed.returncode))
            if completed.returncode != 0:
                print(f"scenario{number}: 失败（退出码 {completed.returncode}）", flush=True)
                break
    finally:
        if owned_root and not args.keep:
            shutil.rmtree(parent, ignore_errors=True)

    print("\n=== summary ===")
    for number, status in statuses:
        print(f"scenario{number}: {'PASS' if status == 0 else 'FAIL'} (exit {status})")
    return 0 if statuses and all(status == 0 for _, status in statuses) else 1


if __name__ == "__main__":
    raise SystemExit(main())

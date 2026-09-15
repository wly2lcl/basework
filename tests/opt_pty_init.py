#!/usr/bin/env python3
"""OPT-001: 在真实 PTY 中验证交互式 init 仍可完成。"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import atexit
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
sys.path.insert(0, str(HERE / "tui_pty"))
from tui_drive import TuiSession  # noqa: E402


def main() -> int:
    root = Path(tempfile.mkdtemp(prefix="basework-opt001-pty-"))
    atexit.register(lambda: shutil.rmtree(root, ignore_errors=True))
    binary = root / "bin" / "basework"
    home = root / "home"
    cache = root / "go-cache"
    binary.parent.mkdir(parents=True)
    home.mkdir()
    go = shutil.which("go")
    if not go:
        print("go not found", file=sys.stderr)
        return 1
    env = dict(os.environ)
    env.update({
        "HOME": str(home),
        "GOCACHE": str(cache),
        "GOTOOLCHAIN": "local",
        "TERM": "xterm-256color",
    })
    for key in ("OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY",
                "OPENCODE_API_KEY", "OG_API_KEY"):
        env.pop(key, None)
    build = subprocess.run(
        [go, "build", "-tags", "sqlite memory", "-o", str(binary), "./cmd/basework"],
        cwd=REPO, env=env, capture_output=True, text=True, check=False,
    )
    if build.returncode != 0:
        print(build.stderr or build.stdout, file=sys.stderr)
        return 1

    session = TuiSession([str(binary), "init"], env=env, cwd=str(REPO), cols=120, rows=32)
    session.start()
    try:
        ok, _ = session.wait_text("请输入编号", timeout=20, tag="provider prompt")
        if not ok:
            raise RuntimeError("交互 init 未显示 provider 提示")
        session.type_line("1")
        ok, _ = session.wait_text("API Key", timeout=20, tag="key prompt")
        if not ok:
            raise RuntimeError("交互 init 未显示 key 提示")
        session.type_line("")
        ok, _ = session.wait_text("请输入编号 (1-", timeout=20, tag="free model prompt")
        if not ok:
            raise RuntimeError("交互 init 未显示免费模型提示")
        session.type_line("")
        ok, _ = session.wait_text("默认模型", timeout=20, tag="model prompt")
        if not ok:
            raise RuntimeError("交互 init 未显示模型结果")
        ok, _ = session.wait_text("初始化完成", timeout=20, tag="init complete")
        if not ok:
            raise RuntimeError("交互 init 未完成")
        session.pump(0.5)
        config_path = home / ".config" / "basework" / "config.json"
        config = json.loads(config_path.read_text(encoding="utf-8"))
        if config.get("provider") != "opencode":
            raise RuntimeError(f"交互 init provider 错误: {config}")
        print("OPT-001 PTY acceptance passed: provider=opencode config_written=true")
        return 0
    finally:
        session.stop()
        shutil.rmtree(root, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())

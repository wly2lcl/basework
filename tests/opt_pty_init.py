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


def selected_go_toolchain(go: str) -> tuple[Path, str, str]:
    """在隔离 HOME 前解析项目选用的 SDK，保留已有工具链和模块缓存。"""
    try:
        probe = subprocess.run(
            [go, "env", "-json", "GOROOT", "GOMODCACHE"], cwd=REPO,
            capture_output=True, text=True, check=False, timeout=60,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise RuntimeError(f"发现项目 Go 工具链失败: {exc}") from exc
    if probe.returncode != 0:
        raise RuntimeError("发现项目 Go 工具链失败: " + (probe.stderr or probe.stdout).strip())
    try:
        settings = json.loads(probe.stdout)
        go_root = settings["GOROOT"]
        modcache = settings["GOMODCACHE"]
        if not isinstance(go_root, str) or not go_root or not isinstance(modcache, str) or not modcache:
            raise ValueError("GOROOT 或 GOMODCACHE 为空")
    except (KeyError, TypeError, ValueError) as exc:
        raise RuntimeError(f"项目 Go 工具链信息无效: {exc}") from exc
    selected_go = Path(go_root) / "bin" / "go"
    if not selected_go.is_file():
        raise RuntimeError(f"项目选用的 Go SDK 不存在: {selected_go}")
    return selected_go, go_root, modcache


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
    try:
        selected_go, go_root, modcache = selected_go_toolchain(go)
    except RuntimeError as exc:
        print(str(exc), file=sys.stderr)
        return 1
    env = dict(os.environ)
    env.update({
        "HOME": str(home),
        "GOROOT": go_root,
        "GOMODCACHE": modcache,
        "PATH": str(selected_go.parent) + os.pathsep + env.get("PATH", ""),
        "GOCACHE": str(cache),
        "GOTOOLCHAIN": "local",
        "TERM": "xterm-256color",
    })
    # PTY 驱动会合并宿主环境，必须用空值覆盖，防止凭证被重新带入子进程。
    for key in ("OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY",
                "AGNES_API_KEY", "OPENCODE_API_KEY", "OG_API_KEY"):
        env[key] = ""
    build = subprocess.run(
        [str(selected_go), "build", "-tags", "sqlite memory", "-o", str(binary), "./cmd/basework"],
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

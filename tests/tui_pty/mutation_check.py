#!/usr/bin/env python3
"""QA-001 负向门禁：移除 session 绑定接线时，场景 6 必须失败。"""

import os
import shutil
import subprocess
import tempfile
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]


def main() -> int:
    root = Path(tempfile.mkdtemp(prefix="basework-qa-mutation-"))
    mutated = root / "repo"
    binary = root / "basework-mutated"
    run_root = root / "scenario6"
    try:
        shutil.copytree(
            REPO,
            mutated,
            ignore=shutil.ignore_patterns(".git", "dist", ".DS_Store", "__pycache__"),
        )
        source = mutated / "cmd" / "basework" / "tui.go"
        original = source.read_text(encoding="utf-8")
        needle = "opts.SessionID = sessionID"
        if original.count(needle) != 1:
            raise RuntimeError("session 绑定变异点不唯一")
        source.write_text(original.replace(needle, "opts.SessionID = \"\""), encoding="utf-8")

        env = dict(os.environ)
        env.setdefault("GOTOOLCHAIN", "local")
        env["GOCACHE"] = str(root / "go-cache")
        build = subprocess.run(
            ["go", "build", "-tags", "sqlite memory", "-o", str(binary), "./cmd/basework"],
            cwd=mutated,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        if build.returncode != 0:
            raise RuntimeError("变异二进制构建失败:\n" + (build.stderr or build.stdout))

        scenario_env = dict(env)
        scenario_env.update({
            "SHIP003_ROOT": str(run_root),
            "SHIP003_BIN": str(binary),
            "SHIP003_SKIP_BUILD": "1",
        })
        result = subprocess.run(
            ["python3", str(REPO / "tests" / "tui_pty" / "scenario6.py")],
            cwd=REPO,
            env=scenario_env,
            check=False,
        )
        if result.returncode == 0:
            raise RuntimeError("session 绑定被移除后 scenario6 仍通过，负向门禁失效")
        print("QA-001 mutation acceptance passed: session binding mutation was rejected")
        return 0
    finally:
        shutil.rmtree(root, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())

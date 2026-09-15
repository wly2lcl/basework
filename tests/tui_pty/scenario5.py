#!/usr/bin/env python3
"""场景 5：嵌入 Go 服务。

可观察的成功结果（路线图口径）：把 basework 当库嵌进仓库外 Go module——
provider/agent/session 全部走公共 API，一次运行完成 read → edit → bash → 答复，
断言磁盘产物、回调事件与会话落盘。

运行体是 examples/embed/main.go（仓库内），本脚本只负责起假服务、
准备隔离环境并运行它、报告结果。
"""

import json
import os
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import harness as H  # noqa: E402

EMBED_SRC = os.path.join(H.REPO_ROOT, "examples", "embed")


def main():
    rep = H.Report("S5", "嵌入 Go 服务：公共 API 组装 + 工具执行 + 事件流 + 会话落盘")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s5-requests.jsonl")
    rep.set("fake_port", port)
    try:
        home = os.path.join(H.ROOT, "home", "s5")
        if os.path.isdir(home):
            shutil.rmtree(home)

        # 复制到仓库外临时 module；这样导入 internal/runtime 会在这里直接编译失败。
        external_src = os.path.join(H.ROOT, "external-embed")
        os.makedirs(external_src, exist_ok=True)
        shutil.copy2(os.path.join(EMBED_SRC, "main.go"),
                     os.path.join(external_src, "main.go"))
        with open(os.path.join(external_src, "go.mod"), "w", encoding="utf-8") as fh:
            fh.write("module example.com/basework-embed-check\n\n"
                     "go 1.26\n\n"
                     "require github.com/wly2lcl/basework v0.0.0\n\n"
                     "replace github.com/wly2lcl/basework => " + H.REPO_ROOT + "\n")

        # 构建仓库外的嵌入示例。
        go = H.go_env(home)
        build = subprocess.run(
            [os.path.join(H.GO_ROOT, "bin", "go"), "build",
             "-o", os.path.join(H.ROOT, "embed-check"), "."],
            cwd=external_src,
            capture_output=True, text=True,
            env={**os.environ, **go, "GOFLAGS": "-mod=mod"})
        rep.check("仓库外 Go module 仅用 pkg 公共 API 可构建", build.returncode == 0,
                  build.stderr.strip()[:200] if build.returncode else "")
        if build.returncode != 0:
            rep.save("s5")
            return 1

        env = {**os.environ, **go}
        p = subprocess.run([os.path.join(H.ROOT, "embed-check"), str(port),
                            os.path.join(H.WORK, "fixbug"), home],
                           capture_output=True, text=True, env=env, timeout=180)
        rep.set("embed_stdout", p.stdout.strip())
        rep.set("embed_stderr", p.stderr.strip()[:400])
        rep.check("嵌入服务验收程序退出码为 0", p.returncode == 0,
                  p.stdout.strip()[:120] or p.stderr.strip()[:120])

        out = p.stdout
        rep.check("嵌入路径完成工具调用并收到运行事件",
                  "工具调用=3" in out and "tool.started" in out and "turn.finished" in out,
                  out.strip()[:160])
        rep.check("嵌入路径的会话事实已持久化",
                  "事件落盘=" in out and not out.endswith("事件落盘=0 条"), out.strip()[-40:])

        recs = H.read_fake_log(log)
        rep.check("嵌入路径向模型端点发出了请求", len(recs) >= 1, f"{len(recs)} 轮")
    finally:
        H.stop_fake_server(proc)
    rep.save("s5")
    print(f"\n场景 5 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

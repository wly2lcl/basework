#!/usr/bin/env python3
"""场景 5：嵌入 Go 服务。

可观察的成功结果（路线图口径）：把 basework 当库嵌进任意 Go 服务——
provider/agent/runtime.NewLocal 全部走公共 API，一次运行完成
read → edit → bash → 答复，断言磁盘产物、事件流与会话落盘。

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

EMBED_SRC = os.path.join(HERE, "..", "..", "examples", "embed")


def main():
    rep = H.Report("S5", "嵌入 Go 服务：公共 API 组装 + 工具执行 + 事件流 + 会话落盘")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s5-requests.jsonl")
    rep.set("fake_port", port)
    try:
        home = os.path.join(H.ROOT, "home", "s5")
        if os.path.isdir(home):
            shutil.rmtree(home)

        # 构建嵌入示例（在仓库 examples/embed 下）。
        go = H.go_env(home)
        build = subprocess.run(
            [os.path.join(H.GO_ROOT, "bin", "go"), "build",
             "-o", os.path.join(H.ROOT, "embed-check"), EMBED_SRC],
            capture_output=True, text=True,
            env={**os.environ, **go, "GOFLAGS": "-mod=mod"})
        rep.check("examples/embed 可独立构建", build.returncode == 0,
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
                  "工具调用=3" in out and "run.started" in out and "run.finished" in out,
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

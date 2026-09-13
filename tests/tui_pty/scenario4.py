#!/usr/bin/env python3
"""场景 4：使用不同模型。

可观察的成功结果（路线图口径）：能力来源明确、缺密钥/不支持某能力时明确告知
且不静默关工具。

三段检查：
A. 换模型 gpt-4o（视觉规则=支持）：状态栏显示模型名，unknown 提示里不再含
   vision；请求仍带全部工具（tool_count 不变=工具没有被静默关掉）；对话可用。
B. 换模型 gpt-3.5-turbo（视觉规则=不支持）：`model info` 明确给出 unsupported
   及依据（来源=declared 规则名），TUI 的 unknown 提示同样不含 vision。
C. 缺密钥：不带 key 启动，必须得到明确、可操作的错误，而不是静默失败。
"""

import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def main():
    rep = H.Report("S4", "使用不同模型：能力来源明确 / 缺密钥明确告知 / 工具不被静默关闭")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s4-requests.jsonl")
    rep.set("fake_port", port)
    try:
        cwd = os.path.join(H.WORK, "quick")

        # ---- A. gpt-4o：能力结论升级为 supported，工具齐全 ----
        homeA, envA = H.make_home("s4a", port, model="gpt-4o")
        envA.update(H.go_env(homeA))
        s = H.run_tui(cwd, envA, "s4a", cols=120, rows=40)
        ok, dt = s.wait_text("空闲", timeout=25, tag="A 启动")
        rep.check("A：gpt-4o 启动", ok, f"{dt:.2f}s")
        screen = s.screen.text()
        rep.check("A：状态栏/提示显示模型名 gpt-4o", "gpt-4o" in screen, "见快照")
        rep.check("A：unknown 提示不再含 vision（能力来源=家族规则）",
                  "vision" not in screen, "vision 已有明确结论")
        s.snapshot("A 首屏")

        s.type_line("[S4] 你是谁？")
        ok, dt = s.wait_text("工具没有被悄悄关掉", timeout=60, tag="A 答复")
        rep.check("A：换模型后对话可用", ok, f"{dt:.2f}s")
        s.key("CTRL_C")
        s.pump(1.0)
        s.stop()
        s.save(H.RUNS, "s4a")

        recs = H.read_fake_log(log)
        tool_counts = [r.get("tool_count") for r in recs if r.get("scenario") == "S4"]
        rep.set("s4_tool_counts", tool_counts)
        rep.check("A：请求仍携带全部工具（未静默关闭）",
                  bool(tool_counts) and min(tool_counts) >= 20,
                  f"tool_count={tool_counts}")

        # ---- B. gpt-3.5-turbo：不支持的视觉能力必须明确说“不支持”并给依据 ----
        genvB = dict(os.environ)
        genvB.update(H.go_env(os.path.join(H.ROOT, "home", "s4a")))
        p = subprocess.run([H.BIN, "model", "info", "gpt-3.5-turbo"],
                           cwd=cwd, capture_output=True, text=True, env=genvB)
        outB = p.stdout + p.stderr
        rep.set("model_info_gpt35", outB[:600])
        rep.check("B：model info 明确给出 vision=unsupported",
                  "vision" in outB and "unsupported" in outB, "CLI 口径")
        rep.check("B：unsupported 带可追溯依据（规则名）",
                  "gpt-3.5" in outB and "纯文本" in outB, "来源=declared 规则")

        homeB, envB = H.make_home("s4b", port, model="gpt-3.5-turbo")
        envB.update(H.go_env(homeB))
        s = H.run_tui(cwd, envB, "s4b", cols=120, rows=40)
        ok, dt = s.wait_text("空闲", timeout=25, tag="B 启动")
        rep.check("B：gpt-3.5-turbo 启动（不支持的是 vision，不是工具）", ok, f"{dt:.2f}s")
        screenB = s.screen.text()
        rep.check("B：unknown 提示不含 vision（它已是明确结论）",
                  "vision" not in screenB, "见快照")
        s.snapshot("B 首屏")
        s.key("CTRL_C")
        s.pump(1.0)
        s.stop()

        # ---- C. 缺密钥：必须明确告知，不能静默 ----
        homeC, envC = H.make_home("s4c", port, model="gpt-4o", with_key=False)
        envC.update(H.go_env(homeC))
        s = H.run_tui(cwd, envC, "s4c", cols=120, rows=40)
        s.pump(5.0)
        screenC = s.screen.text()
        s.snapshot("C 缺密钥首屏")
        told = ("key" in screenC.lower()) or ("密钥" in screenC)
        rep.check("C：缺密钥时明确告知（屏幕出现 key/密钥字样）", told, "见快照 C")
        alive = s.alive()
        if alive:
            s.key("CTRL_C")
            s.pump(1.0)
        s.stop()
        rep.set("tui_alive_without_key", alive)
    finally:
        H.stop_fake_server(proc)
    rep.save("s4")
    print(f"\n场景 4 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

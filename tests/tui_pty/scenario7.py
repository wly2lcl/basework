#!/usr/bin/env python3
"""场景 7：真实 TUI 权限审批的允许/拒绝路径。"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def main():
    rep = H.Report("S7", "TUI 权限审批：真实路径/diff 展示、允许与拒绝")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s7-requests.jsonl")
    try:
        extra = {"permission": {"enabled": True, "mode": "interactive"}}
        home, env = H.make_home("s7", port, extra=extra)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "fixbug")

        # 拒绝路径：首个审批拒绝后不应改盘。
        deny = H.run_tui(cwd, env, "s7-deny", cols=120, rows=46)
        deny.wait_text("空闲", timeout=25, tag="拒绝场景启动")
        deny.type_line("[S1] 请修复 calc.go")
        ok, dt = deny.wait_text("权限确认", timeout=45, tag="拒绝弹窗")
        rep.check("拒绝场景显示权限确认弹窗", ok, f"{dt:.2f}s")
        # 标题和详情由不同的 Bubble Tea 更新到 PTY；只等到标题会偶发
        # 在详情尚未绘制时读取旧屏幕，造成无意义的 flaky 失败。
        tool_ok, _ = deny.wait_text("工具:", timeout=5, tag="拒绝弹窗工具详情")
        path_ok, _ = deny.wait_text("路径:", timeout=5, tag="拒绝弹窗路径详情")
        rep.check("弹窗展示工具和路径证据", tool_ok and path_ok,
                  "见拒绝场景屏幕")
        deny.type_text("n")
        deny.pump(1.0)
        unchanged = open(os.path.join(cwd, "calc.go"), encoding="utf-8").read()
        rep.check("拒绝后文件未修改", "return a - b" in unchanged)
        deny.stop()
        rep.check("拒绝场景由夹具回收且无残留", not deny.alive())

        # 允许路径：每次工具请求都必须显式批准，直到脚本完成。
        H.reset_work()
        home, env = H.make_home("s7-allow", port, extra=extra)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "fixbug")
        allow = H.run_tui(cwd, env, "s7-allow", cols=120, rows=46)
        allow.wait_text("空闲", timeout=25, tag="允许场景启动")
        allow.type_line("[S1] 请修复 calc.go 并测试")
        approvals = 0
        saw_diff = False
        for _ in range(8):
            ok, _ = allow.wait_text("权限确认", timeout=15, tag="允许弹窗")
            if not ok:
                break
            approvals += 1
            screen = allow.screen.text()
            if "预览:" in screen or "calc.go" in screen:
                saw_diff = True
            allow.type_text("y")
            allow.pump(0.3)
            if "已修复 calc.go" in allow.screen.text():
                break
        rep.check("允许路径至少完成一次审批", approvals > 0, f"{approvals} 次")
        rep.check("允许弹窗展示真实路径或预览", saw_diff)
        rep.check("允许后得到完成答复", "已修复 calc.go" in allow.screen.text())
        result = open(os.path.join(cwd, "calc.go"), encoding="utf-8").read()
        rep.check("允许后文件已修改", "return a + b" in result)
        allow.wait_text("运行完成", timeout=20, tag="允许运行收尾")
        allow.key("CTRL_C")
        allow.pump(0.8)
        if allow.alive():
            allow.key("CTRL_C")
            allow.pump(0.8)
        rep.check("允许场景退出无残留", not allow.alive())
        allow.stop()
        allow.save(H.RUNS, "s7")
    finally:
        H.stop_fake_server(proc)
    rep.save("s7")
    print(f"\n场景 7 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

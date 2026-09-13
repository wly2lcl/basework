#!/usr/bin/env python3
"""场景 2：执行耗时检查。

可观察的成功结果（路线图口径）：命令状态与输出持续可见，取消后子进程退出，
结果可回看。
"""

import json
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402

SHELL_PID = "/private/tmp/ship003/runs/slow-shell.pid"
TEST_PID = "/private/tmp/ship003/runs/slow-test.pid"


def read_pid(path):
    for _ in range(120):
        if os.path.exists(path):
            try:
                v = open(path, encoding="utf-8").read().strip()
                if v:
                    return v
            except OSError:
                pass
        time.sleep(0.1)
    return ""


def main():
    rep = H.Report("S2", "执行耗时检查：状态与输出可见 / 取消后子进程退出 / 结果可回看")
    H.reset_work()
    for f in (SHELL_PID, TEST_PID):
        if os.path.exists(f):
            os.remove(f)
    proc, port, log = H.start_fake_server(log_name="s2-requests.jsonl")
    rep.set("fake_port", port)
    try:
        home, env = H.make_home("s2", port)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "slowtest")

        s = H.run_tui(cwd, env, "s2", cols=120, rows=40)
        ok, dt = s.wait_text("空闲", timeout=25, tag="启动")
        rep.check("真实终端里启动并渲染状态栏", ok, f"{dt:.2f}s")

        s.type_line("[S2] 请跑一次耗时的完整检查")
        ok, dt = s.wait_text("已在后台启动", timeout=60, tag="启动答复")
        rep.check("模型确认后台任务已启动", ok, f"{dt:.2f}s")

        shell_pid = read_pid(SHELL_PID)
        test_pid = read_pid(TEST_PID)
        rep.check("后台命令真的在跑（shell PID 可读）", bool(shell_pid), f"pid={shell_pid}")
        rep.check("测试子进程真的在跑（go test 内 PID 可读）", bool(test_pid), f"pid={test_pid}")
        rep.set("shell_pid", shell_pid)
        rep.set("test_pid", test_pid)
        rep.check("取消前两个进程都存活",
                  H.pid_alive(shell_pid) and H.pid_alive(test_pid))

        # ---- 任务卡片：状态可见 ----
        s.key("CTRL_J")
        ok, dt = s.wait_text("[运行中]", timeout=20, tag="任务卡片")
        rep.check("Ctrl+J 打开任务卡片并显示「[运行中]」文字标记", ok, f"{dt:.2f}s")
        s.snapshot("任务卡片")
        card = s.screen.text()
        rep.check("卡片显示命令内容（用户知道在跑什么）",
                  "sh -c" in card or "MARKER-BEFORE" in card,
                  "命令前缀可见（卡片宽度内截断属正常渲染）")

        # ---- 输出可读 ----
        s.type_text("o")
        s.pump(0.3)
        ok, dt = s.wait_text("MARKER-BEFORE", timeout=25, tag="任务输出")
        rep.check("o 读取任务输出，能看到命令已产出的内容", ok, f"{dt:.2f}s")
        s.snapshot("任务输出")

        # ---- 取消 ----
        s.type_text("x")
        s.pump(0.3)
        ok, dt = s.wait_for(lambda t: "[已取消]" in t or "[已中断]" in t or "[失败" in t,
                            timeout=25, tag="取消")
        rep.check("x 取消后卡片转为终态文字标记（非「运行中」）", ok, f"{dt:.2f}s")
        s.snapshot("取消后")
        after = s.screen.text()
        rep.check("取消后卡片不再显示「运行中」", "[运行中]" not in after)

        # ---- 子进程清理（关键：不是只改界面状态） ----
        deadline = time.time() + 20
        while time.time() < deadline and (H.pid_alive(shell_pid) or H.pid_alive(test_pid)):
            time.sleep(0.25)
        rep.check("取消后 shell 进程已退出（子进程清理）", not H.pid_alive(shell_pid), f"pid={shell_pid}")
        rep.check("取消后 go test 进程已退出（进程组清理）", not H.pid_alive(test_pid), f"pid={test_pid}")

        s.key("ESC")
        s.pump(0.5)
        s.key("CTRL_C")
        s.pump(1.0)
        rep.check("退出 TUI 无残留", not s.alive())
        s.stop()
        s.save(H.RUNS, "s2")

        # ---- 结果可回看：退出后仍能查历史 ----
        genv = H.go_env(home)
        p = subprocess.run([H.BIN, "jobs", "list"], cwd=cwd, capture_output=True, text=True,
                           env={**os.environ, **genv})
        out = p.stdout + p.stderr
        rep.set("jobs_list", out.strip()[:400])
        rep.check("退出后 basework jobs list 仍可回看该任务",
                  "canceled" in out and "sh -c" in out,
                  "历史里能看到该任务及终态 canceled")
        rep.check("回看的历史记录里该任务为终态（不含 running）",
                  "running" not in out.lower(), "历史里不残留 running")

        # ---- 不自动重跑 ----
        recs = H.read_fake_log(log)
        rep.check("未发生自动重跑（脚本只被推进到第 2 轮）", len(recs) == 2, f"实际 {len(recs)} 轮")
    finally:
        H.stop_fake_server(proc)
    rep.save("s2")
    print(f"\n场景 2 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

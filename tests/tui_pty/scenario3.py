#!/usr/bin/env python3
"""场景 3：中断后继续。

可观察的成功结果（路线图口径）：重启同会话、恢复事实与摘要、
已终止任务不显示运行中、不自动重跑副作用命令。

做法：第一段会话里读文件 + 起一个长后台任务，然后在任务运行中 SIGKILL 硬杀
TUI（模拟崩溃，不给任何收尾机会）；重启后断言——历史还在、遗留任务被标为
interrupted 而非 running、且重启本身没有触发任何新的模型请求（不自动重跑）。
最后追问一句，确认会话真的可以继续。
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def main():
    rep = H.Report("S3", "中断后继续：重启同会话 / 恢复事实与摘要 / 遗留任务不显示运行中 / 不自动重跑")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s3-requests.jsonl")
    rep.set("fake_port", port)
    try:
        home, env = H.make_home("s3", port)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "quick")

        # ---- 第一段：读文件 + 长后台任务，任务运行中硬杀 ----
        s = H.run_tui(cwd, env, "s3a", cols=120, rows=40)
        ok, dt = s.wait_text("空闲", timeout=25, tag="第一段启动")
        rep.check("第一段：真实终端里启动", ok, f"{dt:.2f}s")

        s.type_line("[S3] 请读一下 main.go，然后跑一次完整检查")
        ok, dt = s.wait_text("长检查", timeout=60, tag="第一段答复")
        rep.check("第一段：读取文件并启动后台长检查", ok, f"{dt:.2f}s")

        # 等任务真正进入 running（读后台任务卡片）再杀，模拟"用户跑了很久之后崩溃"。
        s.type_text("j")  # 若 Ctrl+J 与某绑定冲突，用面板键兜底；先开卡片
        s.pump(0.5)
        s.key("CTRL_J")
        ok, dt = s.wait_text("[运行中]", timeout=15, tag="运行中确认")
        rep.check("第一段：后台任务处于「运行中」", ok, f"{dt:.2f}s")
        s.snapshot("硬杀前：任务运行中")
        s.key("ESC")
        s.pump(0.3)

        killed = s.kill_hard()
        rep.check("第一段：TUI 被 SIGKILL 硬杀（模拟崩溃）", killed)
        phase1_rounds = len(H.read_fake_log(log))
        rep.set("phase1_rounds", phase1_rounds)

        # ---- 第二段：重启同工作区 ----
        s2 = H.run_tui(cwd, env, "s3b", cols=120, rows=40)
        ok, dt = s2.wait_text("空闲", timeout=25, tag="第二段启动")
        rep.check("第二段：重启后启动", ok, f"{dt:.2f}s")
        s2.pump(1.5)

        screen = s2.screen.text()
        s2.snapshot("重启后首屏")
        rep.check("重启后恢复会话历史（能看到上一段的用户消息或痕迹）",
                  "[S3]" in screen or "main.go" in screen or "切换到会话" in screen,
                  "见快照 重启后首屏")

        # 遗留任务：通过 CLI 历史核对状态（面板在空闲时也可能可见）。
        genv = H.go_env(home)
        import subprocess
        p = subprocess.run([H.BIN, "jobs", "list"], cwd=cwd, capture_output=True, text=True,
                           env={**os.environ, **genv})
        out = p.stdout + p.stderr
        rep.set("jobs_list_after_restart", out.strip()[:400])
        rep.check("遗留任务被标记为 interrupted（不是 running）",
                  "interrupted" in out.lower(), "CLI 历史口径")
        rep.check("重启后首屏与任务面板都没有「运行中」残留",
                  "[运行中]" not in screen, "面板未打开时以首屏为准")

        # 不自动重跑：重启 + 空闲一段时间后，模型请求轮数不得增加。
        time.sleep(4.0)
        s2.pump(0.5)
        rounds_now = len(H.read_fake_log(log))
        rep.check("重启后未自动重跑（模型请求数不变）",
                  rounds_now == phase1_rounds,
                  f"硬杀前 {phase1_rounds} 轮，重启后 {rounds_now} 轮")

        # ---- 继续对话：会话真的可用 ----
        s2.type_line("[S3R] 我刚才做到哪了？")
        ok, dt = s2.wait_text("重启后的一次确认", timeout=60, tag="恢复后追问")
        rep.check("恢复后的会话可以继续对话并得到答复", ok, f"{dt:.2f}s")

        s2.key("CTRL_C")
        s2.pump(1.0)
        rep.check("第二段：退出 TUI 无残留", not s2.alive())
        s2.stop()
        s2.save(H.RUNS, "s3b")
    finally:
        H.stop_fake_server(proc)
    rep.save("s3")
    print(f"\n场景 3 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

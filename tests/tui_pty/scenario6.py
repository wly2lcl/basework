#!/usr/bin/env python3
"""场景 6：TUI 运行中切换历史会话。

验证 /session <id> 是真实产品入口：新 runtime 成功绑定后才切换，历史消息
来自选中会话，旧会话消息不串入新屏幕，旧会话的服务资源可以退出。
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def session_ids(home):
    return sorted(name[:-len(".jsonl")] for name in os.listdir(H.session_dir(home))
                  if name.endswith(".jsonl") and name != "jobs.jsonl")


def main():
    rep = H.Report("S6", "TUI 运行中切换历史会话：历史消息与运行资源按 ID 隔离")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s6-requests.jsonl")
    sessions = []
    try:
        home, env = H.make_home("s6", port)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "quick")

        first = H.run_tui(cwd, env, "s6-first", cols=120, rows=40)
        ok, dt = first.wait_text("空闲", timeout=25, tag="第一会话启动")
        rep.check("第一会话启动", ok, f"{dt:.2f}s")
        first.type_line("[S4] SESSION-ONE-UNIQUE")
        ok, dt = first.wait_text("另一个模型", timeout=45, tag="第一会话答复")
        rep.check("第一会话得到答复", ok, f"{dt:.2f}s")
        first.wait_text("运行完成", timeout=20, tag="第一会话收尾")
        ids = session_ids(home)
        rep.check("第一会话已落盘", len(ids) == 1, str(ids))
        if len(ids) != 1:
            raise RuntimeError("无法确定第一会话 ID")
        first_id = ids[0]
        first.key("CTRL_C")
        first.pump(0.8)
        rep.check("第一会话退出无残留", not first.alive())
        first.stop()

        second = H.run_tui(cwd, env, "s6-second", cols=120, rows=40)
        ok, dt = second.wait_text("空闲", timeout=25, tag="第二会话启动")
        rep.check("第二会话启动", ok, f"{dt:.2f}s")
        second.type_line("[S4] SESSION-TWO-UNIQUE")
        ok, dt = second.wait_text("另一个模型", timeout=45, tag="第二会话答复")
        rep.check("第二会话得到答复", ok, f"{dt:.2f}s")
        second.wait_text("运行完成", timeout=20, tag="第二会话收尾")
        ids = session_ids(home)
        rep.check("第二会话已落盘", len(ids) == 2, str(ids))
        if len(ids) != 2:
            raise RuntimeError("无法确定第二会话 ID")
        second_id = next(item for item in ids if item != first_id)
        rep.set("first_session", first_id)
        rep.set("second_session", second_id)

        # 负向门禁：未知 ID 必须报错且保持当前会话，不能静默新建会话。
        missing_id = "f" * 32
        second.type_line("/session " + missing_id)
        ok, dt = second.wait_text("会话 " + missing_id + " 不存在", timeout=20, tag="未知会话拒绝")
        rep.check("未知会话 ID 明确拒绝", ok, f"{dt:.2f}s")
        bad_screen = second.screen.text()
        rep.check("未知会话拒绝后仍停留在原会话",
                  second_id[:12] in bad_screen and len(session_ids(home)) == 2,
                  "当前会话 ID/会话文件数量未变化")

        second.type_line("/session " + first_id)
        ok, dt = second.wait_text("已恢复会话", timeout=45, tag="切换完成")
        rep.check("/session 命令完成真实切换", ok, f"{dt:.2f}s")
        screen = second.screen.text()
        second.snapshot("切换到第一会话后")
        rep.check("切换后展示第一会话 ID", first_id[:12] in screen, "见快照")
        rep.check("切换后恢复第一会话历史", "SESSION-ONE-UNIQUE" in screen, "见快照")
        rep.check("切换后不串入第二会话历史", "SESSION-TWO-UNIQUE" not in screen, "见快照")

        second.key("CTRL_C")
        second.pump(0.8)
        rep.check("切换后的会话退出无残留", not second.alive())
        second.stop()
        second.save(H.RUNS, "s6")
    finally:
        H.stop_fake_server(proc)
    rep.save("s6")
    print(f"\n场景 6 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

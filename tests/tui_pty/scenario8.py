#!/usr/bin/env python3
"""场景 8：真实产品路径触发两次压缩后重启继续。

配置把上下文预算压到可观察范围，驱动真实 basework 二进制连续发起三轮请求，
关闭后按同一 session ID 重启并发起第四轮。断言落在 JSONL 事件与假服务真实
请求内容上：必须存在两次 compacted 事件，且重启请求仍看到第三轮标记。
"""

import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def read_events(path):
    events = []
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                events.append(json.loads(line))
    return events


def main():
    rep = H.Report("S8", "真实产品路径：两次压缩、关闭重启、同会话继续")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s8-requests.jsonl")
    rep.set("fake_port", port)
    try:
        extra = {
            "max_context_tokens": 40,
            "compaction": {
                "enabled": True,
                "strategy": "sliding_window",
                "threshold": 0.5,
                "window_size": 4,
            },
        }
        home, env = H.make_home("s8", port, extra=extra)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "quick")

        s = H.run_tui(cwd, env, "s8a", cols=120, rows=40)
        ok, dt = s.wait_text("空闲", timeout=25, tag="启动")
        rep.check("第一段真实 TUI 启动", ok, f"{dt:.2f}s")
        for marker in ("C8-1", "C8-2", "C8-3"):
            s.type_line(f"[{marker}] 继续处理上下文")
            ok, dt = s.wait_text(f"{marker} 已完成", timeout=45, tag=marker)
            rep.check(f"第一段 {marker} 完成", ok, f"{dt:.2f}s")
        s.key("CTRL_C")
        s.pump(1.0)
        if s.alive():
            # Ctrl+C 在运行态承担取消语义；若终端仍处于空闲态，Ctrl+D
            # 是无歧义的退出键，避免把取消与退出混成同一个断言。
            s.key("CTRL_D")
            s.pump(1.0)
        rep.check("第一段退出无残留", not s.alive())
        s.stop()

        session_files = [name for name in os.listdir(H.session_dir(home))
                         if name.endswith(".jsonl") and name != "jobs.jsonl"]
        rep.check("同一工作区只生成一个会话", len(session_files) == 1, str(session_files))
        if len(session_files) != 1:
            raise RuntimeError("无法唯一确定压缩场景会话 ID")
        sid = session_files[0][:-len(".jsonl")]
        rep.set("session_id", sid)
        session_path = os.path.join(H.session_dir(home), session_files[0])
        events = read_events(session_path)
        compacted = [e for e in events if e.get("type") == "compacted"]
        rep.set("compacted_events_before_restart", len(compacted))
        rep.check("关闭前真实产品已持久化至少一次压缩", len(compacted) >= 1,
                  f"compacted={len(compacted)}")

        s2 = H.run_tui(cwd, env, "s8b", cols=120, rows=40,
                       argv=[H.BIN, "tui", "--session", sid])
        ok, dt = s2.wait_text("空闲", timeout=25, tag="重启")
        rep.check("按同一 session ID 重启", ok and sid[:12] in s2.screen.text(), f"{dt:.2f}s")
        s2.type_line("[C8-4] 重启后继续")
        ok, dt = s2.wait_text("C8-4 已完成", timeout=45, tag="C8-4")
        rep.check("重启后真实产品继续对话", ok, f"{dt:.2f}s")
        s2.key("CTRL_C")
        s2.pump(1.0)
        if s2.alive():
            s2.key("CTRL_D")
            s2.pump(1.0)
        rep.check("第二段退出无残留", not s2.alive())
        s2.stop()

        events = read_events(session_path)
        compacted = [e for e in events if e.get("type") == "compacted"]
        rep.set("compacted_events_after_restart", len(compacted))
        rep.check("重启前后累计两次真实压缩事件", len(compacted) >= 2,
                  f"compacted={len(compacted)}")

        recs = H.read_fake_log(log)
        resumed = [r for r in recs if r.get("scenario") == "C8"
                    and r.get("history_markers", {}).get("[C8-3]")]
        rep.check("重启请求实际看到第三轮历史标记", bool(resumed), str(recs[-2:]))
    finally:
        H.stop_fake_server(proc)
    rep.save("s8")
    print(f"\n场景 8 结论：{'通过' if rep.passed() else '未通过'}")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    raise SystemExit(main())

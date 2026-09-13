#!/usr/bin/env python3
"""场景 1：修复一个缺陷（真实终端 + 脚本化模型）。

可观察的成功结果（路线图口径）：展示涉及文件、修改前后差异、测试结果；
失败有可操作说明。
"""

import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import harness as H  # noqa: E402


def main():
    rep = H.Report("S1", "修复一个缺陷：涉及文件 / 修改前后差异 / 测试结果")
    H.reset_work()
    proc, port, log = H.start_fake_server(log_name="s1-requests.jsonl")
    rep.set("fake_port", port)
    try:
        home, env = H.make_home("s1", port)
        env.update(H.go_env(home))
        cwd = os.path.join(H.WORK, "fixbug")
        rep.set("workdir", cwd)

        s = H.run_tui(cwd, env, "s1", cols=120, rows=46)
        ok, dt = s.wait_text("空闲", timeout=25, tag="启动")
        rep.check("真实终端里启动并渲染状态栏", ok, f"{dt:.2f}s")
        s.snapshot("启动")
        rep.check("状态栏显示模型与 Provider（能力来源可见）",
                  "ship003-fake" in s.screen.text() and "openai" in s.screen.text())

        s.type_line("[S1] 请修复 calc.go 里 Add 的缺陷，并跑测试确认")
        t_enter = time.time()
        ok, dt = s.wait_for(lambda t: "请修复" in t and "缺陷" in t, timeout=20, tag="用户消息")
        rep.check("用户消息进入界面且中文/空格未被截断", ok, f"{dt:.2f}s")
        s.snapshot("输入后")

        # 分阶段观察：每一步都必须「在屏幕上真的出现过」，而不是只看终态。
        stages = [
            ("read 卡片", "🔧 read({", 60),
            ("edit_files 预览卡片", 'edit_files({"action": "preview"', 60),
        ]
        observed = []
        for label, needle, to in stages:
            ok, dt = s.wait_text(needle, timeout=to, tag=label)
            rep.check(f"阶段可见：{label}", ok, f"{dt:.2f}s")
            observed.append((label, ok, dt))
            s.snapshot(label)

        # 预演差异必须真的显示改动前后两行。
        ok, dt = s.wait_for(lambda t: "-return a - b" in t and "+return a + b" in t,
                            timeout=60, tag="差异")
        rep.check("提交前差异可见（旧行/新行都在屏上）", ok, f"{dt:.2f}s")
        s.snapshot("差异")

        ok, dt = s.wait_for(lambda t: 'edit_files({"action": "commit"' in t, timeout=60, tag="commit")
        rep.check("阶段可见：edit_files 提交卡片", ok, f"{dt:.2f}s")
        observed.append(("edit_files 提交卡片", ok, dt))
        s.snapshot("commit")
        screen_now = s.screen.text()
        rep.check("commit 的 plan_id 来自 preview 的真实返回值",
                  "plan-" in screen_now and "MISSING" not in screen_now,
                  "未出现 MISSING 占位")

        ok, dt = s.wait_for(lambda t: '🔧 bash({"command": "go test ./..."})' in t,
                            timeout=90, tag="bash")
        rep.check("阶段可见：go test 执行卡片", ok, f"{dt:.2f}s")
        observed.append(("go test 执行卡片", ok, dt))
        s.snapshot("bash")

        ok, dt = s.wait_text("已修复 calc.go 的 Add", timeout=90, tag="答复")
        rep.check("阶段可见：最终答复", ok, f"{dt:.2f}s")
        observed.append(("最终答复", ok, dt))
        run_seconds = time.time() - t_enter
        rep.set("run_seconds", round(run_seconds, 2))
        s.pump(1.0)
        final = s.snapshot("最终")

        rep.check("界面上测试未报失败（无「退出码」失败文案）", "退出码" not in final)
        rep.check("最终答复给出了可操作的结论（含文件与验证）",
                  "calc.go" in final and "go test" in final)

        order_ok = all(o[1] for o in observed) and [o[0] for o in observed] == [
            "read 卡片", "edit_files 预览卡片", "edit_files 提交卡片", "go test 执行卡片", "最终答复"]
        rep.check("阶段顺序正确（读取 → 预览 → 提交 → 测试 → 答复）", order_ok,
                  " → ".join(o[0] for o in observed))

        s.key("CTRL_C")
        s.pump(1.0)
        rep.check("Ctrl+C 后进程退出且无残留", not s.alive())
        s.stop()
        raw_path = s.save(H.RUNS, "s1")
        rep.set("transcript", raw_path)

        # ---- 磁盘事实（不采信模型自述）----
        calc = open(os.path.join(cwd, "calc.go"), encoding="utf-8").read()
        rep.check("磁盘 calc.go 已改为 a + b", "return a + b" in calc)
        rep.check("磁盘 calc.go 不再含 a - b", "return a - b" not in calc)
        test = open(os.path.join(cwd, "calc_test.go"), encoding="utf-8").read()
        rep.check("测试文件未被越权改动", "期望 5" in test)

        genv = H.go_env(home)
        p = subprocess.run(["go", "test", "./...", "-count=1"], cwd=cwd, capture_output=True,
                           text=True, env={**os.environ, **genv})
        rep.check("独立执行 go test ./... 通过（夹具侧复核）", p.returncode == 0,
                  (p.stdout + p.stderr).strip().splitlines()[-1] if (p.stdout + p.stderr).strip() else "")

        # ---- 请求侧证据 ----
        recs = H.read_fake_log(log)
        tools = sorted({t for r in recs for t in r.get("tools", [])})
        rep.set("fake_requests", len(recs))
        rep.set("advertised_tools", tools)
        rep.check("模型请求带上了工具定义（工具未被静默关闭）", len(tools) > 0, f"{len(tools)} 个")
        rep.check("全部为流式请求", all(r.get("stream") for r in recs))
        rep.check("多轮迭代按脚本推进（5 轮）", len(recs) == 5, f"实际 {len(recs)} 轮")
    finally:
        H.stop_fake_server(proc)
    rep.save("s1")
    print(f"\n场景 1 结论：{'通过' if rep.passed() else '未通过'}（耗时 {rep.data.get('run_seconds')}s）")
    return 0 if rep.passed() else 1


if __name__ == "__main__":
    sys.exit(main())

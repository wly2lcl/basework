#!/usr/bin/env python3
"""SHIP-003 自动化 TUI 验收的公共装置：假服务 + 隔离 HOME + 会话驱动。"""

import json
import os
import shutil
import signal
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tui_drive import TuiSession  # noqa: E402

# 运行目录可用环境变量覆盖：默认放在系统临时目录，避免污染仓库。
ROOT = os.environ.get("SHIP003_ROOT", "/private/tmp/ship003")
BIN = os.path.join(ROOT, "bin", "basework")
RUNS = os.path.join(ROOT, "runs")
FIXTURES = os.path.join(ROOT, "fixtures")
TEMPLATES = os.path.join(ROOT, "templates")
WORK = os.path.join(ROOT, "work")

# 本机有两个 go：/usr/local/go 实为 go1.24.10，真正的 1.26.0 SDK 在 ~/sdk/go1.26.0。
# GOTOOLCHAIN=auto 时 go 会「切换」到 $HOME/sdk 下已下载的工具链——隔离 HOME 后
# 那个目录不存在，切换就退化成去 PATH 找 go1.26.0（命中 golang.org/dl 桩）并报
# 「not downloaded」。因此夹具必须把真实 SDK 放到 PATH 首位并用 GOTOOLCHAIN=local
# 明确禁止切换：否则测的就不是产品，而是本机工具链解析。
GO_ROOT = "/Users/wangluyao/sdk/go1.26.0"


def go_env(home):
    """夹具与夹具内 `go test` 共用的、确定的 Go 环境。"""
    return {
        "HOME": home,
        "PATH": os.path.join(GO_ROOT, "bin") + ":/usr/bin:/bin:/usr/sbin:/sbin",
        "GOROOT": GO_ROOT,
        "GOTOOLCHAIN": "local",
    }



def reset_work():
    """从模板重建工作区——每次运行都从同一缺陷态出发，保证可复现。"""
    if os.path.isdir(WORK):
        shutil.rmtree(WORK)
    shutil.copytree(FIXTURES, WORK)
    return WORK


def start_fake_server(scenario_script=None, log_name="fake-requests.jsonl"):
    log_path = os.path.join(RUNS, log_name)
    if os.path.exists(log_path):
        os.remove(log_path)
    portfile = os.path.join(ROOT, "fake.port")
    if os.path.exists(portfile):
        os.remove(portfile)
    env = dict(os.environ)
    env["SHIP003_FAKE_LOG"] = log_path
    env["SHIP003_FAKE_PORTFILE"] = portfile
    if scenario_script:
        env["SHIP003_FAKE_SCRIPT"] = scenario_script
    proc = subprocess.Popen(
        [sys.executable, os.path.join(ROOT, "fake_openai.py")],
        env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
        start_new_session=True)
    deadline = time.time() + 15
    while time.time() < deadline:
        if os.path.exists(portfile):
            break
        if proc.poll() is not None:
            err = proc.stderr.read().decode(errors="replace")
            raise RuntimeError("假服务启动失败:\n" + err)
        time.sleep(0.05)
    else:
        raise RuntimeError("假服务未在 15s 内写出端口")
    port = open(portfile, encoding="utf-8").read().strip()
    return proc, int(port), log_path


def stop_fake_server(proc):
    if proc is None:
        return
    try:
        os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
    except (ProcessLookupError, PermissionError):
        try:
            proc.terminate()
        except Exception:
            pass
    try:
        proc.wait(timeout=5)
    except Exception:
        try:
            os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
        except Exception:
            pass


def make_home(home_name, port, model="ship003-fake", provider="openai",
              with_key=True, extra=None):
    """准备隔离 HOME：写 config.json，返回 (home, env)。"""
    home = os.path.join(ROOT, "home", home_name)
    if os.path.isdir(home):
        shutil.rmtree(home)
    cfgdir = os.path.join(home, ".config", "basework")
    os.makedirs(cfgdir, exist_ok=True)

    endpoint = {"base_url": f"http://127.0.0.1:{port}/v1"}
    if with_key:
        endpoint["api_key"] = "ship003-fake-key"
    cfg = {
        "provider": provider,
        "model": model,
        "providers": {provider: endpoint},
    }
    if extra:
        cfg.update(extra)
    with open(os.path.join(cfgdir, "config.json"), "w", encoding="utf-8") as fh:
        json.dump(cfg, fh, ensure_ascii=False, indent=2)

    env = {
        "HOME": home,
        "BASEWORK_BASE_URL": f"http://127.0.0.1:{port}/v1",
    }
    if with_key:
        env["OPENAI_API_KEY"] = "ship003-fake-key"
    return home, env


def session_dir(home):
    return os.path.join(home, ".local", "share", "basework", "sessions")


def run_tui(cwd, env, name, cols=120, rows=40, extra_env=None, argv=None):
    e = dict(env)
    if extra_env:
        e.update(extra_env)
    argv = argv or [BIN, "tui"]
    s = TuiSession(argv, env=e, cwd=cwd, cols=cols, rows=rows, name=name)
    s.start()
    return s


def read_fake_log(path):
    recs = []
    if not os.path.exists(path):
        return recs
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if line:
                try:
                    recs.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
    return recs


def pid_alive(pid):
    try:
        os.kill(int(pid), 0)
    except (ProcessLookupError, ValueError):
        return False
    except PermissionError:
        return True
    return True


class Report:
    """场景报告：逐项断言 + 观察值，最终落 JSON 作为证据。"""

    def __init__(self, scenario, desc):
        self.data = {"scenario": scenario, "desc": desc, "checks": [],
                     "notes": [], "started": time.strftime("%Y-%m-%dT%H:%M:%S")}

    def check(self, name, ok, detail=""):
        self.data["checks"].append({"name": name, "ok": bool(ok), "detail": str(detail)})
        mark = "PASS" if ok else "FAIL"
        print(f"    [{mark}] {name}" + (f" — {detail}" if detail else ""))
        return bool(ok)

    def note(self, text):
        self.data["notes"].append(str(text))

    def set(self, key, value):
        self.data[key] = value

    def passed(self):
        return all(c["ok"] for c in self.data["checks"])

    def save(self, name):
        os.makedirs(RUNS, exist_ok=True)
        path = os.path.join(RUNS, name + ".json")
        self.data["pass"] = self.passed()
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(self.data, fh, ensure_ascii=False, indent=2)
        return path

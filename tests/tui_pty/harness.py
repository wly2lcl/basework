#!/usr/bin/env python3
"""SHIP-003 自动化 TUI 验收的公共装置：假服务 + 隔离 HOME + 会话驱动。"""

import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import atexit

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from tui_drive import TuiSession  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))

# 运行目录可用环境变量覆盖：默认创建一次性临时目录，避免依赖某次历史运行
# 留下的 fixtures 或 PID 文件。显式传入 SHIP003_ROOT 时保留目录，方便复盘报告。
_OWN_ROOT = "SHIP003_ROOT" not in os.environ
ROOT = os.environ.get("SHIP003_ROOT") or tempfile.mkdtemp(prefix="basework-ship003-")
BIN = os.environ.get("SHIP003_BIN") or os.path.join(ROOT, "bin", "basework")
SKIP_BUILD = os.environ.get("SHIP003_SKIP_BUILD") == "1"
RUNS = os.path.join(ROOT, "runs")
FIXTURES = os.path.join(REPO_ROOT, "tests", "tui_pty", "fixtures")
TEMPLATES = os.path.join(ROOT, "templates")
WORK = os.path.join(ROOT, "work")

def _go_root():
    """发现当前工具链；SHIP003_GO_ROOT 可用于 CI 或隔离 SDK。"""
    explicit = os.environ.get("SHIP003_GO_ROOT")
    if explicit:
        return os.path.abspath(explicit)
    go = shutil.which("go")
    if not go:
        raise RuntimeError("未找到 go；可设置 SHIP003_GO_ROOT 指向 SDK")
    result = subprocess.run([go, "env", "GOROOT"], capture_output=True,
                            text=True, check=False)
    if result.returncode != 0 or not result.stdout.strip():
        raise RuntimeError("无法发现 Go SDK: " + result.stderr.strip())
    return result.stdout.strip()


GO_ROOT = _go_root()
_GO = os.path.join(GO_ROOT, "bin", "go")
_GO_MODCACHE = os.environ.get("GOMODCACHE")
if not _GO_MODCACHE:
    _modcache_probe = subprocess.run([_GO, "env", "GOMODCACHE"],
                                     capture_output=True, text=True, check=False)
    _GO_MODCACHE = _modcache_probe.stdout.strip()


def _cleanup_owned_root():
    # 自己创建的临时根只在所有子进程已回收后清理；显式根由调用者保留用于复盘。
    if _OWN_ROOT:
        shutil.rmtree(ROOT, ignore_errors=True)


atexit.register(_cleanup_owned_root)


def go_env(home):
    """夹具与夹具内 `go test` 共用的、确定的 Go 环境。"""
    return {
        "HOME": home,
        "PATH": os.path.join(GO_ROOT, "bin") + ":" + os.environ.get("PATH", "/usr/bin:/bin"),
        "GOROOT": GO_ROOT,
        "GOTOOLCHAIN": "local",
        "GOCACHE": os.path.join(ROOT, "go-cache"),
        **({"GOMODCACHE": _GO_MODCACHE} if _GO_MODCACHE else {}),
    }



def reset_work():
    """从模板重建工作区——每次运行都从同一缺陷态出发，保证可复现。"""
    if os.path.isdir(WORK):
        shutil.rmtree(WORK)
    if not os.path.isdir(FIXTURES):
        raise RuntimeError(f"夹具目录不存在: {FIXTURES}")
    shutil.copytree(FIXTURES, WORK)
    ensure_binary()
    return WORK


def ensure_binary():
    """从当前 checkout 构建验收二进制，避免依赖仓库外旧 bin。"""
    if SKIP_BUILD:
        if not os.path.isfile(BIN):
            raise RuntimeError(f"SHIP003_SKIP_BUILD=1 但二进制不存在: {BIN}")
        return
    os.makedirs(os.path.dirname(BIN), exist_ok=True)
    home = os.path.join(ROOT, "bootstrap-home")
    env = dict(os.environ)
    env.update(go_env(home))
    result = subprocess.run(
        [_GO, "build", "-tags", "sqlite memory", "-o", BIN, "./cmd/basework"],
        cwd=REPO_ROOT, env=env, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        raise RuntimeError("构建 basework 失败:\n" + (result.stderr or result.stdout).strip())


def start_fake_server(scenario_script=None, log_name="fake-requests.jsonl"):
    os.makedirs(RUNS, exist_ok=True)
    log_path = os.path.join(RUNS, log_name)
    if os.path.exists(log_path):
        os.remove(log_path)
    portfile = os.path.join(ROOT, "fake.port")
    if os.path.exists(portfile):
        os.remove(portfile)
    env = dict(os.environ)
    env["SHIP003_FAKE_LOG"] = log_path
    env["SHIP003_FAKE_PORTFILE"] = portfile
    env["SHIP003_RUNS"] = RUNS
    env["SHIP003_REPO_ROOT"] = REPO_ROOT
    if scenario_script:
        env["SHIP003_FAKE_SCRIPT"] = scenario_script
    proc = subprocess.Popen(
        [sys.executable, os.path.join(HERE, "fake_openai.py")],
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

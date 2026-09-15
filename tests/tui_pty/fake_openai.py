#!/usr/bin/env python3
"""确定性 OpenAI 兼容 SSE 假服务（SHIP-003 自动化 TUI 验收专用）。

为什么需要它：TUI 验收要跑真实二进制、真实终端、真实 HTTP provider，
但模型的输出必须可复现，否则「通过」无法区分是产品对了还是模型心情好。
本服务按**脚本**回放响应，并把每个请求的关键事实（模型、是否带 tools、
工具名清单）落盘，作为「工具没有被静默关闭」的可核证据。

协议细节对齐 pkg/provider/openai.go：
- 请求：POST /v1/chat/completions，body 含 model / messages / stream / tools
- 流式：每行 `data: {...}`，以 `data: [DONE]` 结束
- 工具调用：delta.tool_calls[].function.name/arguments，末帧 finish_reason=tool_calls
- 用量：末尾一帧 {"choices":[],"usage":{...}}
"""

import json
import os
import re
import shlex
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

DEFAULT_ROOT = os.path.join(tempfile.gettempdir(), "basework-ship003")
LOG_PATH = os.environ.get("SHIP003_FAKE_LOG",
                          os.path.join(DEFAULT_ROOT, "runs", "fake-requests.jsonl"))
RUNS_PATH = os.environ.get("SHIP003_RUNS", os.path.dirname(LOG_PATH))
# 模拟模型与网络延迟，让 TUI 的中间态可被观察（默认 0.35s / 0.08s）。
RESPONSE_DELAY = float(os.environ.get("SHIP003_FAKE_DELAY", "0.35"))
CHUNK_DELAY = float(os.environ.get("SHIP003_FAKE_CHUNK_DELAY", "0.08"))


def load_scenarios():
    """从 SCRIPT 文件读场景脚本；缺省用内置。"""
    path = os.environ.get("SHIP003_FAKE_SCRIPT", "")
    if path and os.path.exists(path):
        with open(path, encoding="utf-8") as fh:
            return json.load(fh)
    return DEFAULT_SCENARIOS


# 每个场景是一串「步」。步里有 tool（工具调用）或 text（文本回复）。
# 第几步由请求里已出现的 tool 结果消息条数决定——这使脚本与 agent 的
# 多轮迭代天然对齐，不需要服务端保存会话状态。
DEFAULT_SCENARIOS = {
    # 场景 1：修复一个缺陷。
    # 走产品设计的可审阅编辑流程：preview（出差异，不写盘）→ commit（带验证命令）。
    # commit 用的 plan_id 由 preview 的返回推导，服务端从上一轮工具结果里取
    # （占位符 $PLANID），因此不依赖对派发算法的硬编码。
    "S1": {
        "steps": [
            {"tool": "read", "args": {"path": "calc.go"}},
            {"tool": "edit_files", "args": {"action": "preview", "edits": [
                {"path": "calc.go", "old": "return a - b", "new": "return a + b"}]}},
            {"tool": "edit_files", "args": {"action": "commit", "plan_id": "$PLANID",
                                            "verify": "go test ./..."}},
            {"tool": "bash", "args": {"command": "go test ./..."}},
            {"text": "已修复 calc.go 的 Add：`a - b` → `a + b`。"
                     "先预览差异再提交，提交后独立执行 `go test ./...`，全部通过。"},
        ]
    },
    # 场景 2：执行耗时检查（后台任务 + 取消 + 输出可回看）。
    # 命令刻意打印一个标记（证明输出可读）并记录两个 PID：shell 的与测试进程的，
    # 这样「取消后子进程退出」能分开验证进程组清理是否彻底。
    "S2": {
        "steps": [
            {"tool": "bash_background",
             "args": {"command": "sh -c 'echo MARKER-BEFORE; echo $$ > " + shlex.quote(os.path.join(RUNS_PATH, "slow-shell.pid")) + "; "
                                "SHIP003_PIDFILE=" + shlex.quote(os.path.join(RUNS_PATH, "slow-test.pid")) + " "
                                "go test ./... -count=1 -run TestSlowCheck -timeout 600s -v; "
                                "echo MARKER-AFTER'"}},
            {"text": "耗时检查已在后台启动。用 Ctrl+J 打开任务卡片，o 读取输出，x 取消。"},
        ]
    },
    # 场景 3：中断后继续。第一段脚本：读文件 + 起一个长后台任务（TUI 随后被硬杀）。
    # 第二段（重启后用户追问）：只回文本——用于验证会话上下文确实恢复。
    "S3": {
        "steps": [
            {"tool": "read", "args": {"path": "main.go"}},
            {"tool": "bash_background",
             "args": {"command": "sleep 300"}},
            {"text": "已读取 main.go，并记录事实 FACT-S3-ORCHARD；在后台启动了一次长检查。"},
        ]
    },
    "S3R": {
        "steps": [
            {"text": "收到。这是重启后的一次确认：我已看到上次会话遗留的事实 FACT-S3-ORCHARD。"},
        ]
    },
    # 场景 4：不同模型。只回文本；工具是否被静默关闭由请求日志里的 tool_count 判定。
    "S4": {
        "steps": [
            {"text": "我是另一个模型，工具没有被悄悄关掉——这条回复证明请求链路正常。"},
        ]
    },
    # 嵌入式 Go 服务场景：read → edit → bash → 文本（走 builtin 工具集）。
    "SE": {
        "steps": [
            {"tool": "read", "args": {"path": "calc.go"}},
            {"tool": "edit", "args": {"path": "calc.go",
                                      "old": "return a - b", "new": "return a + b"}},
            {"tool": "bash", "args": {"command": "go test ./..."}},
            {"text": "嵌入服务已修复 calc.go 并通过 go test 验证。"},
        ]
    },
    # 越界写入：触发权限/路径校验拒绝
    "S5": {
        "steps": [
            {"tool": "write",
             "args": {"path": os.path.join(RUNS_PATH, "should-not-exist.txt"), "content": "越界写入"}},
            {"text": "写入被拒绝，我没有重试。"},
        ]
    },
}


SCENARIOS = load_scenarios()
LOG_LOCK = threading.Lock()


def scenario_for(messages):
    """取最近一条用户消息里的 [Sx] 标记作为场景键。

    会话恢复场景会把上一段 [S3] 一并带回请求；若从头扫描，会把恢复追问
    错当成第一段脚本，导致夹具无法验证同一会话的历史。
    """
    for m in reversed(messages):
        if m.get("role") != "user":
            continue
        content = m.get("content")
        text = ""
        if isinstance(content, str):
            text = content
        elif isinstance(content, list):
            text = " ".join(p.get("text", "") for p in content if isinstance(p, dict))
        for token in ("[C8-4]", "[C8-3]", "[C8-2]", "[C8-1]",
                      "[S5]", "[SE]", "[S1]", "[S2]", "[S3R]", "[S3]", "[S4]"):
            if token in text:
                return "C8" if token.startswith("[C8-") else token[1:-1]
        return "S3"
    return "S3"


def completed_tool_rounds(messages):
    """数出请求里已有的工具结果条数 = 已完成的工具轮次。"""
    return sum(1 for m in messages if m.get("role") == "tool")


def sse(obj):
    return "data: " + json.dumps(obj, ensure_ascii=False) + "\n\n"


def last_tool_content(messages):
    for m in reversed(messages):
        if m.get("role") == "tool":
            c = m.get("content")
            if isinstance(c, str):
                return c
            if isinstance(c, list):
                return " ".join(p.get("text", "") for p in c if isinstance(p, dict))
    return ""


def message_text(message):
    content = message.get("content")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return " ".join(p.get("text", "") for p in content if isinstance(p, dict))
    return ""


def resolve_placeholders(args, messages):
    """把 $PLANID 替换成上一轮工具结果里出现的 plan id。

    edit_files 的 plan_id 由 Op 内容派生（derivePlanID），服务端不该复制该算法——
    复制就等于把实现细节焊进夹具，产品改了夹具却还「通过」。改为从真实返回值读取。
    """
    if not isinstance(args, dict):
        return args
    if "$PLANID" not in json.dumps(args, ensure_ascii=False):
        return args
    content = last_tool_content(messages)
    m = re.search(r"plan_id[=: ]+([A-Za-z0-9_\-]+)", content)
    plan = m.group(1) if m else "MISSING"
    out = json.loads(json.dumps(args, ensure_ascii=False).replace("$PLANID", plan))
    return out


def chunks_for(step, model, messages):
    """把一个脚本步翻译成 SSE 帧序列。"""
    out = []
    if "tool" in step:
        name = step["tool"]
        args = json.dumps(resolve_placeholders(step.get("args", {}), messages),
                          ensure_ascii=False)        # 参数刻意分两片发送，验证客户端按增量拼接（REL-001 语义）。
        half = max(1, len(args) // 2)
        out.append(sse({"choices": [{"index": 0, "delta": {"role": "assistant",
                       "tool_calls": [{"index": 0, "id": "call_1", "type": "function",
                                       "function": {"name": name, "arguments": args[:half]}}]}}]}))
        out.append(sse({"choices": [{"index": 0, "delta": {"tool_calls": [
                       {"index": 0, "function": {"arguments": args[half:]}}]}}]}))
        out.append(sse({"choices": [{"index": 0, "delta": {}, "finish_reason": "tool_calls"}]}))
    else:
        text = step.get("text", "")
        # 切成小片，制造真实的流式增量。
        for i in range(0, len(text), 12):
            out.append(sse({"choices": [{"index": 0, "delta": {"content": text[i:i + 12]}}]}))
        out.append(sse({"choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]}))
    out.append(sse({"choices": [], "usage": {"prompt_tokens": 120,
                                             "completion_tokens": 40,
                                             "total_tokens": 160}}))
    out.append("data: [DONE]\n\n")
    return out


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):  # 静音默认访问日志
        pass

    def do_GET(self):
        if self.path.endswith("/models"):
            body = json.dumps({"object": "list", "data": [
                {"id": "ship003-fake", "object": "model"}]}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n)
        try:
            req = json.loads(raw)
        except Exception:
            req = {}

        messages = req.get("messages") or []
        scenario = scenario_for(messages)
        step_idx = completed_tool_rounds(messages)
        steps = SCENARIOS.get(scenario, {}).get("steps", [])

        tool_names = sorted({t.get("function", {}).get("name")
                             for t in (req.get("tools") or [])})
        rec = {
            "scenario": scenario,
            "model": req.get("model"),
            "stream": req.get("stream"),
            "tool_count": len(tool_names),
            "tools": tool_names,
            "tool_rounds_done": step_idx,
            "path": self.path,
            "has_auth": bool(self.headers.get("Authorization")),
        }
        if scenario == "C8":
            # 压缩/重启场景不接受固定答复作为恢复证据：把每轮标记是否
            # 仍存在于真实请求中的历史消息写入脱敏日志，驱动脚本据此核对。
            rec["history_markers"] = {
                marker: any(marker in message_text(m) for m in messages)
                for marker in ("[C8-1]", "[C8-2]", "[C8-3]")
            }
            current = next((message_text(m) for m in reversed(messages)
                            if m.get("role") == "user"), "")
            if "[C8-1]" in current:
                step = {"text": "C8-1 已完成：读取并记录第一段上下文。"}
            elif "[C8-2]" in current:
                step = {"text": "C8-2 已完成：继续处理第二段上下文。"}
            elif "[C8-3]" in current:
                step = {"text": "C8-3 已完成：压缩前保留第三段上下文。"}
            else:
                step = {"text": "C8-4 已完成：重启后继续，最近上下文仍可见。"}
        elif scenario == "S3R":
            # 只允许在历史消息中确实包含第一段写入的唯一事实时确认恢复。
            # 当前追问本身不带该 token，避免夹具只检查模型固定文案。
            current_user = max((i for i, m in enumerate(messages)
                                if m.get("role") == "user"), default=len(messages))
            history = "\n".join(message_text(m) for m in messages[:current_user])
            rec["history_marker_seen"] = "FACT-S3-ORCHARD" in history
            if not rec["history_marker_seen"]:
                step = {"text": "恢复失败：历史中没有 FACT-S3-ORCHARD，不能确认已恢复。"}
            else:
                step = SCENARIOS.get(scenario, {}).get("steps", [])[0]
        else:
            step = None
        with LOG_LOCK:
            with open(LOG_PATH, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(rec, ensure_ascii=False) + "\n")

        if step is None:
            if step_idx >= len(steps):
                step = {"text": "（脚本已用尽）"}
            else:
                step = steps[step_idx]

        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "close")
        self.end_headers()
        # 人为加入模型延迟与分片间隔：真实模型不是 0 延迟返回的，
        # 没有延迟就观察不到「流式增量 → 工具卡片 → 最终答复」的中间态，
        # 自动化测试只能在终态上打勾（等于没验证过程可见性）。
        time.sleep(RESPONSE_DELAY)
        frames = chunks_for(step, req.get("model"), messages)
        for i, frame in enumerate(frames):
            self.wfile.write(frame.encode())
            self.wfile.flush()
            if CHUNK_DELAY and frame.startswith("data: {") and '"content"' in frame:
                time.sleep(CHUNK_DELAY)
            elif CHUNK_DELAY and i == 0:
                time.sleep(CHUNK_DELAY)

    def handle_one_request(self):
        try:
            super().handle_one_request()
        except (BrokenPipeError, ConnectionResetError):
            pass


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 0
    os.makedirs(os.path.dirname(LOG_PATH), exist_ok=True)
    srv = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    # 把实际端口写到文件，供驱动脚本读取（避免端口竞争）。
    with open(os.environ.get("SHIP003_FAKE_PORTFILE",
                             os.path.join(DEFAULT_ROOT, "fake.port")),
              "w", encoding="utf-8") as fh:
        fh.write(str(srv.server_address[1]))
    srv.serve_forever()


if __name__ == "__main__":
    main()

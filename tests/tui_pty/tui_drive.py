#!/usr/bin/env python3
"""TUI 自动化驱动：在真实 PTY 里跑真实二进制，按键、抓屏、断言。

区别于「调用 View() 看字符串」的单元测试，这里走完整终端路径：
- 真 PTY：raw mode / 备用屏 / 窗口尺寸都真实生效
- 真按键：写的是终端字节序列（Enter = \\r，Ctrl+J = 0x0a，Tab = \\t）
- 真抓屏：内嵌一个最小 VT 解析器，把 ANSI 流渲染成字符网格，
  因此断言的是「用户此刻在屏幕上看到什么」，而不是源码里的字符串。
"""

import fcntl
import os
import pty
import re
import select
import signal
import struct
import sys
import termios
import time
import codecs
import unicodedata

# CSI 语法：参数字节 0x30–0x3F，中间字节 0x20–0x2F，终止字节 0x40–0x7E。
CSI_RE = re.compile(r"\x1b\[([\x30-\x3f]*)([\x20-\x2f]*)([\x40-\x7e])")
CSI_PARTIAL_RE = re.compile(r"\x1b\[[\x30-\x3f]*[\x20-\x2f]*$")
OSC_RE = re.compile(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)")


# --------------------------------------------------------------------------
# 最小 VT 解析器：只实现 bubbletea 用到的子集
# --------------------------------------------------------------------------

class Screen:
    """把 ANSI 字节流渲染成 rows x cols 的字符网格。

    三个必须做对的点（做错了会「测出」不存在的问题）：
    1. **UTF-8 跨读边界**：必须用增量解码器，否则一个汉字被切成两半，
       后续字节整体错位，屏幕上会凭空少字。
    2. **CSI 参数范围**：参数字节是 0x30–0x3F（含 `> < = ?`），中间字节
       0x20–0x2F，终止字节 0x40–0x7E。只认 `[0-9;?]` 会把 `ESC[>4m`
       当成普通文本写进屏幕（曾观察到字面量 `[>4m`）。
    3. **宽字符**：CJK 占两列，占位必须占住第二格，否则按列定位的
       重绘会落在错误的列上。
    """

    def __init__(self, cols, rows):
        self.cols = cols
        self.rows = rows
        self.grid = [[" "] * cols for _ in range(rows)]
        self.r = 0
        self.c = 0
        self.saved = (0, 0)
        self._dec = codecs.getincrementaldecoder("utf-8")("replace")
        self._txt = ""       # 未消费完的转义序列 / 半个序列

    # ---- 基础写入 ----
    def _clear(self):
        self.grid = [[" "] * self.cols for _ in range(self.rows)]
        self.r = self.c = 0

    @staticmethod
    def _width(ch):
        return 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1

    def _put(self, ch):
        w = self._width(ch)
        if self.c + w > self.cols:
            self.c = 0
            self.r += 1
            self._scroll_if_needed()
        if 0 <= self.r < self.rows and 0 <= self.c < self.cols:
            self.grid[self.r][self.c] = ch
            if w == 2 and self.c + 1 < self.cols:
                self.grid[self.r][self.c + 1] = ""   # 宽字符占位
        self.c += w

    def _scroll_if_needed(self):
        while self.r >= self.rows:
            self.grid.pop(0)
            self.grid.append([" "] * self.cols)
            self.r = self.rows - 1

    # ---- 转义序列处理 ----
    def _csi(self, params, final):
        nums = []
        for p in params.split(";"):
            p = p.strip()
            if p.startswith("?"):
                p = p[1:]
            try:
                nums.append(int(p) if p else 0)
            except ValueError:
                nums.append(0)
        n0 = nums[0] if nums else 0

        if final in "Hf":
            self.r = max(0, min(self.rows - 1, (nums[0] or 1) - 1))
            self.c = max(0, min(self.cols - 1, (nums[1] if len(nums) > 1 and nums[1] else 1) - 1))
        elif final == "A":
            self.r = max(0, self.r - (n0 or 1))
        elif final == "B":
            self.r = min(self.rows - 1, self.r + (n0 or 1))
        elif final == "C":
            self.c = min(self.cols - 1, self.c + (n0 or 1))
        elif final == "D":
            self.c = max(0, self.c - (n0 or 1))
        elif final == "G":
            self.c = max(0, min(self.cols - 1, (n0 or 1) - 1))
        elif final == "d":
            self.r = max(0, min(self.rows - 1, (n0 or 1) - 1))
        elif final == "J":
            if n0 == 2 or n0 == 3:
                self._clear()
            elif n0 == 0:
                for cc in range(self.c, self.cols):
                    self.grid[self.r][cc] = " "
                for rr in range(self.r + 1, self.rows):
                    self.grid[rr] = [" "] * self.cols
            elif n0 == 1:
                for cc in range(0, self.c + 1):
                    self.grid[self.r][cc] = " "
                for rr in range(0, self.r):
                    self.grid[rr] = [" "] * self.cols
        elif final == "K":
            if n0 == 0:
                for cc in range(self.c, self.cols):
                    self.grid[self.r][cc] = " "
            elif n0 == 1:
                for cc in range(0, self.c + 1):
                    self.grid[self.r][cc] = " "
            else:
                self.grid[self.r] = [" "] * self.cols
        elif final == "L":
            for _ in range(n0 or 1):
                self.grid.insert(self.r, [" "] * self.cols)
                self.grid.pop()
        elif final == "M":
            for _ in range(n0 or 1):
                self.grid.pop(self.r)
                self.grid.append([" "] * self.cols)
        elif final == "P":
            row = self.grid[self.r]
            del row[self.c:self.c + (n0 or 1)]
            row.extend([" "] * ((n0 or 1)))
            self.grid[self.r] = row[:self.cols]
        elif final in "hl":
            # 私有模式：1049/47/1047 = 备用屏（清屏），其余忽略
            if "?" in params and n0 in (1047, 1049, 47):
                if final == "h":
                    self._clear()
        elif final in "mrtTsu":
            pass  # SGR / 滚动 / 保存光标等：不影响字符网格内容

    def feed(self, data: bytes):
        self._txt += self._dec.decode(data)
        i = 0
        n = len(self._txt)
        while i < n:
            ch = self._txt[i]
            if ch == "\x1b":
                if i + 1 >= n:
                    break                       # 序列未完，等下一块
                nxt = self._txt[i + 1]
                if nxt == "[":
                    m = CSI_RE.match(self._txt, i)
                    if m:
                        self._csi(m.group(1) + m.group(2), m.group(3))
                        i = m.end()
                        continue
                    if CSI_PARTIAL_RE.match(self._txt, i):
                        break                   # 参数还没收完
                    i += 1                      # 畸形序列：丢弃 ESC
                    continue
                if nxt == "]":
                    m = OSC_RE.match(self._txt, i)
                    if m:
                        i = m.end()
                        continue
                    i += 1
                    continue
                if nxt in "()":
                    i += 3 if i + 2 < n else 1
                    continue
                if nxt in "78":
                    self.saved = (self.r, self.c) if nxt == "7" else self.saved
                    if nxt == "8":
                        self.r, self.c = self.saved
                    i += 2
                    continue
                if nxt == "M":
                    self.r = max(0, self.r - 1)
                    i += 2
                    continue
                i += 2
                continue
            if ch == "\r":
                self.c = 0
            elif ch == "\n":
                self.r += 1
                self._scroll_if_needed()
            elif ch == "\b":
                self.c = max(0, self.c - 1)
            elif ch == "\t":
                self.c = min(self.cols - 1, (self.c // 8 + 1) * 8)
            elif ch in "\x07\x00\x0e\x0f":
                pass
            else:
                self._put(ch)
            i += 1
        self._txt = self._txt[i:]
        # 防御：畸形流不能让缓冲无界增长
        if len(self._txt) > 4096:
            self._txt = ""

    def text(self):
        return "\n".join("".join(row).rstrip() for row in self.grid).strip("\n")


# --------------------------------------------------------------------------
# PTY 会话
# --------------------------------------------------------------------------

class TuiSession:
    """在一个真实 PTY 里运行子进程并驱动它。"""

    KEYS = {
        "ENTER": "\r", "TAB": "\t", "ESC": "\x1b", "SPACE": " ",
        "CTRL_C": "\x03", "CTRL_D": "\x04", "CTRL_J": "\x0a", "CTRL_L": "\x0c",
        "CTRL_P": "\x10", "BACKSPACE": "\x7f",
        "UP": "\x1b[A", "DOWN": "\x1b[B", "RIGHT": "\x1b[C", "LEFT": "\x1b[D",
    }

    def __init__(self, argv, env=None, cwd=None, cols=120, rows=40, name="tui"):
        self.argv = argv
        self.env = env or {}
        self.cwd = cwd or os.getcwd()
        self.cols, self.rows = cols, rows
        self.name = name
        self.pid = None
        self.fd = None
        self.raw = bytearray()
        self.screen = Screen(cols, rows)
        self.snapshots = []          # (t, 屏幕文本)
        self.t0 = None
        self.exit_status = None

    # ---- 生命周期 ----
    def start(self):
        env = dict(os.environ)
        env.update(self.env)
        env["TERM"] = "xterm-256color"
        env["COLUMNS"] = str(self.cols)
        env["LINES"] = str(self.rows)
        # 清掉可能影响判定的继承变量
        for k in ("BASEWORK_BASE_URL", "BASEWORK_PROVIDER"):
            if k not in self.env:
                env.pop(k, None)

        pid, fd = pty.fork()
        if pid == 0:                      # 子进程
            try:
                os.chdir(self.cwd)
                os.execvpe(self.argv[0], self.argv, env)
            except Exception as exc:      # pragma: no cover
                os.write(2, f"exec 失败: {exc}\n".encode())
                os._exit(127)
        self.pid, self.fd = pid, fd
        self.t0 = time.time()
        self.resize(self.cols, self.rows)
        return self

    def resize(self, cols, rows):
        self.cols, self.rows = cols, rows
        self.screen.cols, self.screen.rows = cols, rows
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ,
                    struct.pack("HHHH", rows, cols, 0, 0))

    def _read_available(self, timeout):
        try:
            r, _, _ = select.select([self.fd], [], [], timeout)
        except (OSError, ValueError):
            return False
        if not r:
            return False
        try:
            data = os.read(self.fd, 65536)
        except OSError:
            return False
        if not data:
            return False
        self.raw.extend(data)
        self.screen.feed(data)
        return True

    def pump(self, seconds):
        """持续读取一段时间，把输出喂进屏幕缓冲。"""
        end = time.time() + seconds
        while time.time() < end:
            self._read_available(min(0.1, max(0.0, end - time.time())))

    def wait_for(self, pred, timeout=30.0, poll=0.15, tag=""):
        """等到屏幕上出现满足 pred 的内容。返回 (是否命中, 耗时)。"""
        start = time.time()
        end = start + timeout
        while time.time() < end:
            if pred(self.screen.text()):
                return True, time.time() - start
            self._read_available(poll)
        hit = pred(self.screen.text())
        if not hit:
            self.snapshots.append((time.time() - self.t0, self.screen.text()))
        return hit, time.time() - start

    def wait_text(self, needle, timeout=30.0, tag=""):
        return self.wait_for(lambda t: needle in t, timeout=timeout, tag=tag or needle)

    def snapshot(self, label=""):
        self.snapshots.append((time.time() - self.t0, self.screen.text()))
        return self.screen.text()

    # ---- 输入 ----
    def send(self, text):
        os.write(self.fd, text.encode())

    def key(self, name, times=1):
        for _ in range(times):
            self.send(self.KEYS[name])
            self.pump(0.12)

    def type_line(self, text):
        """逐字输入再回车——贴一整串会让输入组件难以处理。"""
        for ch in text:
            self.send(ch)
            self.pump(0.02)
        self.key("ENTER")

    def type_text(self, text):
        for ch in text:
            self.send(ch)
            self.pump(0.02)

    # ---- 退出 ----
    def stop(self, grace=4.0):
        if self.pid is None:
            return
        for sig in (signal.SIGTERM, signal.SIGKILL):
            try:
                os.kill(self.pid, sig)
            except ProcessLookupError:
                break
            deadline = time.time() + grace
            while time.time() < deadline:
                try:
                    p, st = os.waitpid(self.pid, os.WNOHANG)
                except ChildProcessError:
                    self.pid = None
                    return
                if p:
                    self.exit_status = st
                    self._read_available(0.2)
                    self.pid = None
                    return
                self._read_available(0.05)
        self.pid = None

    def kill_hard(self):
        """SIGKILL 立即杀死 TUI——模拟崩溃/断电，不给它任何收尾机会。
        返回进程是否真的被杀掉（True=之前还活着）。"""
        if self.pid is None:
            return False
        try:
            os.kill(self.pid, signal.SIGKILL)
        except ProcessLookupError:
            self.pid = None
            return False
        deadline = time.time() + 5
        while time.time() < deadline:
            try:
                p, st = os.waitpid(self.pid, os.WNOHANG)
            except ChildProcessError:
                self.pid = None
                return True
            if p:
                self.exit_status = st
                self._read_available(0.2)
                self.pid = None
                return True
            self._read_available(0.05)
        return True

    def alive(self):
        if self.pid is None:
            return False
        try:
            p, _ = os.waitpid(self.pid, os.WNOHANG)
        except ChildProcessError:
            self.pid = None
            return False
        if p:
            self.pid = None
            return False
        return True

    # ---- 证据 ----
    def save(self, outdir, prefix=None):
        prefix = prefix or self.name
        os.makedirs(outdir, exist_ok=True)
        with open(os.path.join(outdir, prefix + ".raw"), "wb") as fh:
            fh.write(self.raw)
        with open(os.path.join(outdir, prefix + ".screen.txt"), "w", encoding="utf-8") as fh:
            fh.write(self.screen.text() + "\n")
        with open(os.path.join(outdir, prefix + ".snapshots.txt"), "w", encoding="utf-8") as fh:
            for t, txt in self.snapshots:
                fh.write(f"===== t+{t:6.2f}s =====\n{txt}\n\n")
        return os.path.join(outdir, prefix + ".raw")


def strip_ansi(data: bytes) -> str:
    text = data.decode("utf-8", errors="replace")
    text = re.sub(r"\x1b\[[0-9;?]*[A-Za-z@]", "", text)
    text = re.sub(r"\x1b\][^\x07]*\x07", "", text)
    return text.replace("\r", "\n")


if __name__ == "__main__":
    # 冒烟：把命令跑在 PTY 里并打印抓屏
    argv = sys.argv[1:]
    s = TuiSession(argv, cols=100, rows=30).start()
    s.pump(3)
    print(s.screen.text())
    s.stop()

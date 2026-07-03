# TUI 使用指南

## 概述

TUI（Terminal User Interface）提供完整的终端编程助手体验，基于 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建。相比简单 REPL 模式，TUI 支持 Markdown 渲染、语法高亮、命令面板、会话管理等高级特性。

> **注意**：TUI 当前为开发中功能（Phase 18），尚未正式发布。

## 启动 TUI

### 基本启动

```bash
basework tui
```

### 指定主题

```bash
basework tui --theme dark
basework tui --theme light
basework tui --theme dracula
basework tui --theme monokai
```

### 恢复会话

```bash
basework tui --resume <session-id>
```

### 回退到简单模式

```bash
basework agent --no-tui
```

## 界面布局

```
┌─────────────────────────────────────────┐
│ Status Bar: 模型 | Provider | Tokens    │ ← 顶部状态栏
├─────────────────────────────────────────┤
│                                         │
│  消息区域（Markdown 渲染）              │ ← 中间消息区
│  - 用户消息                             │
│  - 助手响应                             │
│  - 工具调用                             │
│                                         │
├─────────────────────────────────────────┤
│ > 输入区（多行支持）                    │ ← 底部输入区
└─────────────────────────────────────────┘
```

### 顶部状态栏

状态栏显示当前会话的关键信息：

- **模型名**：当前使用的 AI 模型
- **Provider**：模型提供商（如 Anthropic、OpenAI）
- **Token 使用**：当前会话的 token 计数
- **会话 ID**：当前会话标识符
- **MCP 连接状态**：已连接/断开
- **忙/闲状态**：Agent 是否正在处理请求

### 中间消息区

消息区按时间顺序展示对话历史：

- **用户消息**：以对话气泡形式显示，带有时间戳
- **助手响应**：Markdown 渲染，支持代码块语法高亮
- **工具调用**：显示工具名称、参数和返回结果
- **错误消息**：红色高亮显示

### 底部输入区

输入区支持多行文本输入，带有行号和字符计数：

- 粘贴多行文本自动处理缩进
- 输入 `/` 开头触发命令模式
- 输入 `./` 或 `/` 触发文件路径补全

## 快捷键

### 全局快捷键

| 快捷键 | 说明 |
|--------|------|
| `Ctrl+C` | 取消当前操作 |
| `Ctrl+D` | 退出 TUI |
| `Ctrl+L` | 清屏 |
| `Ctrl+P` | 打开命令面板 |
| `?` | 显示帮助 |

### 输入区快捷键

| 快捷键 | 说明 |
|--------|------|
| `Enter` | 发送消息 |
| `Shift+Enter` | 换行 |
| `↑/↓` | 历史导航 |
| `Tab` | 自动补全（文件路径、命令） |
| `Ctrl+A` | 光标移到行首 |
| `Ctrl+E` | 光标移到行尾 |
| `Ctrl+K` | 删除到行尾 |
| `Ctrl+W` | 删除前一个单词 |
| `Ctrl+U` | 删除整行 |

### 消息区快捷键

| 快捷键 | 说明 |
|--------|------|
| `↑/↓` | 滚动消息 |
| `Home` | 滚动到顶部 |
| `End` | 滚动到底部 |
| `y` | 复制当前消息 |
| `Ctrl+F` | 在消息中搜索 |
| `n` / `N` | 下一个/上一个搜索结果 |

## 配置

### 主题配置

```json
{
  "tui": {
    "enabled": true,
    "theme": "dark",
    "markdown": true,
    "syntax_highlight": true,
    "compact_mode": false,
    "diff_mode": "unified"
  }
}
```

### 主题说明

| 主题 | 说明 |
|------|------|
| `dark` | 深色背景（默认） |
| `light` | 浅色背景 |
| `dracula` | Dracula 主题 |
| `monokai` | Monokai 主题 |

### Diff 模式

```json
{
  "tui": {
    "diff_mode": "unified"
  }
}
```

| 模式 | 说明 |
|------|------|
| `unified` | 统一 diff（单栏） |
| `split` | 分屏 diff（双栏） |

### 紧凑模式

```json
{
  "tui": {
    "compact_mode": true
  }
}
```

紧凑模式减少消息间距和边框，在有限终端空间中显示更多内容。适合小窗口或需要浏览大量上下文时使用。

## 功能特性

### Markdown 渲染

消息区的 Markdown 渲染支持以下元素：

- **代码块**：支持 40+ 语言的语法高亮
- **列表**：有序列表和无序列表
- **链接**：可点击（终端支持的情况下）
- **粗体/斜体**：文本样式
- **表格**：对齐列渲染
- **块引用**：引用样式
- **标题**：H1–H6 层级

### 工具调用显示

工具调用以折叠卡片形式展示，可展开查看详细信息：

```
🔧 bash "ls -la"
   ✅ 成功 (23ms)
   输出: ...
```

调用状态图标：

| 图标 | 含义 |
|------|------|
| 🔧 | 工具正在执行 |
| ✅ | 执行成功 |
| ❌ | 执行失败 |
| ⏳ | 等待执行 |

### 流式响应

- **逐字输出效果**：AI 响应逐 token 渲染，实时可见
- **思考过程显示**：reasoning token 以特殊样式展示（灰/斜体）
- **工具调用进度**：执行中的工具调用显示旋转进度指示器
- **取消响应**：按 `Ctrl+C` 中断当前流式输出

### 自动补全

| 触发方式 | 补全内容 |
|----------|----------|
| 输入 `/` 或 `./` | 文件路径 |
| 输入 `basework` | 子命令 |
| 输入 `--model` 后 | 模型名称 |
| 输入 `/` 开头 | 内置命令 |

## 命令面板

按 `Ctrl+P` 打开命令面板，快速访问常用操作：

```
> _
  session list     - 列出会话
  model switch     - 切换模型
  permission list  - 查看权限
  help             - 显示帮助
  exit             - 退出
```

命令面板支持模糊搜索，输入关键词即可过滤匹配项。选中后按 `Enter` 执行。

### 内置命令

在输入区以 `/` 开头可直接执行以下命令：

| 命令 | 说明 |
|------|------|
| `/session list` | 列出所有会话 |
| `/session switch <id>` | 切换到指定会话 |
| `/session new` | 创建新会话 |
| `/model switch <name>` | 切换模型 |
| `/permission list` | 查看工具权限 |
| `/help` | 显示帮助信息 |
| `/clear` | 清屏 |
| `/exit` | 退出 TUI |

## 会话管理

### 查看会话列表

```bash
# 在 TUI 中
/session list

# 或使用命令面板：Ctrl+P → 选择 session list
```

### 切换会话

```
/session switch <session-id>
```

切换后消息区立即更新为选中会话的历史记录。

### 创建新会话

```
/session new
```

新会话继承当前配置（模型、主题等），可通过命令面板快速切换回旧会话。

### 自动保存

会话自动保存到 `~/.config/basework/sessions/`，重启 TUI 后可通过 `--resume` 恢复。

## 故障排查

### TUI 无法启动

1. 检查终端是否支持 256 色：
   ```bash
   echo $TERM
   ```
   需要 `xterm-256color` 或类似值。

2. 检查终端大小：TUI 需要至少 80×24 字符的终端窗口。

3. 回退到简单模式：
   ```bash
   basework agent --no-tui
   ```

### Markdown 渲染异常

1. 禁用 Markdown 渲染：
   ```json
   {
     "tui": { "markdown": false }
   }
   ```

2. 检查终端编码：
   ```bash
   locale
   ```
   需要支持 UTF-8。

### 快捷键不工作

1. 检查终端模拟器是否有快捷键冲突（如 `Ctrl+P` 在 iTerm2 中默认映射为打印）。
2. 使用命令面板（`Ctrl+P`）替代快捷键操作。
3. 在支持的终端中运行：Kitty、Alacritty、WezTerm、iTerm2、Windows Terminal。

### 渲染性能问题

- 大量消息（>500条）可能影响渲染速度，建议定期使用 `/session new` 创建新会话。
- 启用紧凑模式减少每屏显示的消息密度。
- 使用 `Ctrl+L` 清屏释放内存中的渲染缓存。

## 自定义主题

### 创建主题文件

在 `~/.config/basework/themes/custom.json` 中定义：

```json
{
  "name": "custom",
  "colors": {
    "background": "#1e1e1e",
    "foreground": "#d4d4d4",
    "accent": "#569cd6",
    "error": "#f44747",
    "success": "#6a9955",
    "warning": "#dcdcaa",
    "muted": "#808080"
  },
  "syntax": {
    "keyword": "#569cd6",
    "string": "#ce9178",
    "comment": "#6a9955",
    "function": "#dcdcaa",
    "number": "#b5cea8",
    "type": "#4ec9b0"
  }
}
```

### 颜色字段说明

| 字段 | 说明 |
|------|------|
| `background` | 背景色 |
| `foreground` | 前景色（主文本） |
| `accent` | 强调色（选中项、焦点） |
| `error` | 错误消息 |
| `success` | 成功消息 |
| `warning` | 警告消息 |
| `muted` | 次要文本（时间戳、元信息） |

### 语法高亮颜色

`syntax` 对象控制代码块中各类 token 的颜色，字段名与 TextMate scope 命名一致。

### 使用自定义主题

```bash
basework tui --theme custom
```

主题名称对应 `~/.config/basework/themes/` 目录下的文件名（不含 `.json`）。

## 注意事项

- TUI 需要终端支持 256 色，不支持真彩色（24-bit）的终端可能显示色差。
- 某些 IDE 集成终端（如 VS Code 内置终端）可能不完全支持所有 TUI 功能。
- 性能：大量消息（>500条）可能影响渲染速度，建议定期清理会话。
- 不支持在 tmux 或 screen 中嵌套使用 TUI（可能造成快捷键冲突）。
- 自定义主题文件不合法时，TUI 将回退到默认 `dark` 主题并打印警告。

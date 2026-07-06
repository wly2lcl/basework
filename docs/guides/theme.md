# 主题配置指南

本文档介绍 basework 的 TUI 主题系统，包括内置主题、自定义主题创建和配置方法。

---

## 内置主题

basework 内置 4 种主题，可在配置文件中通过 `theme.name` 切换：

| 主题名 | 风格 | 适合场景 |
|--------|------|----------|
| `dark` | 深色背景，高对比度 | 默认，适合大多数终端 |
| `light` | 浅色背景，柔和色彩 | 亮色终端或日间使用 |
| `dracula` | 紫色调深色主题 | 代码阅读，护眼 |
| `monokai` | 经典 MT 配色 | 开发环境，高辨识度 |

### dark 主题（默认）

深色背景主题，高对比度配色，适合大多数终端环境。

```json
{
  "theme": {
    "name": "dark"
  }
}
```

### light 主题

浅色背景主题，适合亮色终端或日间使用场景。

```json
{
  "theme": {
    "name": "light"
  }
}
```

### dracula 主题

紫色调深色主题，基于 Dracula 官方配色方案，适合长时间编码。

```json
{
  "theme": {
    "name": "dracula"
  }
}
```

### monokai 主题

经典 Monokai 配色，鲜艳色彩对比，辨识度高。

```json
{
  "theme": {
    "name": "monokai"
  }
}
```

---

## 自定义主题

### 自定义主题目录

通过 `theme.custom_path` 指定自定义主题目录，basework 会自动加载其中的主题文件：

```json
{
  "theme": {
    "name": "my-custom-theme",
    "custom_path": "~/.config/basework/themes/"
  }
}
```

### 主题文件格式

自定义主题使用 JSON 格式定义，文件名作为主题名（不含 `.json` 扩展名）。

示例：`~/.config/basework/themes/solarized.json`

```json
{
  "name": "solarized-dark",
  "description": "Solarized dark theme for basework",
  "colors": {
    "background": "#002b36",
    "foreground": "#839496",
    "primary": "#268bd2",
    "secondary": "#2aa198",
    "accent": "#b58900",
    "error": "#dc322f",
    "success": "#859900",
    "warning": "#cb4b16",
    "info": "#268bd2",
    "muted": "#586e75",
    "border": "#073642",
    "selection": "#073642"
  },
  "styles": {
    "status_bar": {
      "background": "#073642",
      "foreground": "#93a1a1"
    },
    "input_area": {
      "background": "#002b36",
      "foreground": "#839496",
      "border": "#586e75"
    },
    "message_user": {
      "background": "#073642",
      "foreground": "#268bd2"
    },
    "message_assistant": {
      "background": "#002b36",
      "foreground": "#839496"
    },
    "message_tool": {
      "background": "#002b36",
      "foreground": "#586e75"
    },
    "error": {
      "foreground": "#dc322f"
    },
    "link": {
      "foreground": "#268bd2",
      "underline": true
    }
  },
  "syntax_highlighting": {
    "keyword": "#859900",
    "string": "#2aa198",
    "number": "#b58900",
    "function": "#268bd2",
    "comment": "#586e75",
    "type": "#b58900",
    "variable": "#b58900"
  }
}
```

### 主题字段说明

| 字段 | 说明 |
|------|------|
| `name` | 主题唯一名称 |
| `description` | 主题描述（可选） |
| `colors.background` | 全局背景色 |
| `colors.foreground` | 全局前景色 |
| `colors.primary` | 主色（用于高亮和强调） |
| `colors.secondary` | 次色 |
| `colors.accent` | 强调色 |
| `colors.error` | 错误消息颜色 |
| `colors.success` | 成功状态颜色 |
| `colors.warning` | 警告消息颜色 |
| `colors.info` | 信息消息颜色 |
| `colors.muted` | 弱化文本颜色 |
| `colors.border` | 边框颜色 |
| `colors.selection` | 选中项背景色 |
| `styles.*` | 各 UI 组件样式覆盖 |
| `syntax_highlighting.*` | 代码语法高亮颜色 |

---

## 配置示例

### 切换主题

```json
{
  "theme": {
    "name": "dracula"
  }
}
```

### 使用自定义主题

```json
{
  "theme": {
    "name": "my-solarized",
    "custom_path": "~/.config/basework/themes/"
  }
}
```

### 完整配置

```json
{
  "theme": {
    "name": "dark",
    "custom_path": "~/.config/basework/themes/"
  }
}
```

---

## 终端自适应

basework 会自动检测终端颜色支持（true color / 256 色 / 16 色）并降级适配：

- **True color** — 完整颜色支持，所有主题颜色精确显示
- **256 色** — 将主题颜色映射到最近的 256 色调色板
- **16 色** — 降级到标准终端色，优先保证可读性

无需额外配置，basework 在启动时自动检测。

---

## 相关文档

- [TUI 使用指南](tui-guide.md) — TUI 启动、快捷键、主题配置
- [配置参考](configuration.md) — 配置文件完整说明
- [性能分析指南](profiling.md) — pprof 集成、benchmark
- [安全配置指南](security.md) — 权限持久化、敏感路径保护
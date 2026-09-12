# pkg/skill

## 用途

基于 Markdown 的技能（Skill）加载。一个 skill 就是一份带 YAML frontmatter 的说明书，
加载后其指令内容可以注入到 agent 的系统提示或工具描述里。

- `Loader` — 跨多个搜索路径发现并管理 skill。
- `Skill` — `Name` / `Description` / `Instructions` / `FilePath` / `Builtin` / `Metadata`。
- `ToPromptXML` — 把若干 skill 渲染成注入提示的 XML 片段。

## 配置

```go
loader := skill.NewLoader(userDir, builtinDir)  // 顺序很重要，见下
loader.Discover()
loader.Active()   // 当前生效的 skill
loader.Get(name)  // 按名取用
```

skill 文件约定为 `SKILL.md`，元数据写在 YAML frontmatter（`name`、`description` 等）。

**路径顺序即优先级**：`paths[0]` 被视为用户目录，其余路径加载到的 skill 会被标记
`Builtin = true`；同名时用户版本胜出（内置版本被丢弃）。

## 扩展点

- 新增 skill：写一个 `SKILL.md` 放进搜索路径即可，不需要改代码。
- 覆盖内置 skill：在用户目录放一个同名 `SKILL.md`，内置版本会自动让位。
- 换注入格式：不用 `ToPromptXML`，直接读 `Instructions` 自行拼装。

## Model Experience

Skill 文本会影响模型指令，但不会自动安装工具或授予权限。模型仍受运行时已注册能力限制；发现失败、跳过和来源规则见下节，不应把文件存在等同加载成功。

## Known Limitations

- **skill 指令是纯文本注入，没有沙箱与权限控制。** 加载第三方 skill 等价于把它的内容
  交给模型执行，来源必须可信。
- **解析失败不返回错误**：格式不合法的 `SKILL.md` 只会打一条 `[skill] warning: skipping
  invalid SKILL.md` 日志然后跳过，调用方拿不到「有文件被跳过」的信号。搜索路径不存在或
  不可访问时同样静默跳过。
- **`Builtin` 的判定依据是路径位置**，不是文件来源：任何「非第一个路径」加载到的 skill
  都会被标为内置。调换 `NewLoader` 的传参顺序会改变这个标记与同名覆盖的胜负。
- 只识别 `SKILL.md` 这一种文件名。
- 没有版本、依赖与冲突解析——同名冲突的解决规则只有「第一个路径优先」一条。

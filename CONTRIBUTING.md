# 贡献指南

感谢你对 Basework 的关注！我们欢迎各种形式的贡献 — 报告问题、提交代码、改进文档。

---

## 报告问题

### 使用 GitHub Issues

- 访问 [github.com/wly2lcl/basework/issues](https://github.com/wly2lcl/basework/issues) 创建新 Issue
- 先搜索已有 Issue，避免重复

### 问题模板

```markdown
**描述**
简明扼要地描述问题。

**复现步骤**
1. 执行命令 `...`
2. 看到错误 `...`

**期望行为**
应该发生什么。

**实际行为**
实际发生了什么。

**环境**
- Go 版本：1.26+
- 操作系统：macOS 14 / Ubuntu 22.04 / Windows 11
- Basework 版本：commit SHA 或 tag
```

### Bug 报告 checklist

- [ ] 提供完整的复现步骤
- [ ] 包含 Go 版本和操作系统信息
- [ ] 附上错误日志或截图
- [ ] 说明是否在干净环境中可复现

---

## 提交代码

### 工作流程

1. **Fork 仓库**

   点击 GitHub 页面右上角的 Fork 按钮。

2. **克隆到本地**

   ```bash
   git clone https://github.com/your-username/basework.git
   cd basework
   ```

3. **创建特性分支**

   ```bash
   git checkout -b feature/amazing-feature
   ```

   分支命名规范：

   | 前缀 | 用途 |
   |------|------|
   | `feature/` | 新功能 |
   | `fix/` | Bug 修复 |
   | `docs/` | 文档改进 |
   | `refactor/` | 代码重构 |
   | `test/` | 测试补充 |
   | `chore/` | 构建/工具变更 |

4. **编写代码和测试**

   - 遵循 Go 标准代码风格（`gofmt`, `go vet`）
   - 所有导出类型/函数必须有中文注释
   - 新功能必须有单元测试（覆盖率目标见 [docs/STATUS.md](docs/STATUS.md)）
   - 确保测试通过

5. **提交代码**

   ```bash
   git add <files>
   git commit -m 'feat: add amazing feature'
   ```

   提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)：

   | 类型 | 说明 |
   |------|------|
   | `feat:` | 新功能 |
   | `fix:` | Bug 修复 |
   | `docs:` | 文档变更 |
   | `refactor:` | 代码重构 |
   | `test:` | 测试相关 |
   | `chore:` | 构建/工具/依赖变更 |
   | `perf:` | 性能优化 |

6. **推送和创建 Pull Request**

   ```bash
   git push origin feature/amazing-feature
   ```

   在 GitHub 上创建 Pull Request，指向 `main` 分支。

### PR checklist

- [ ] 测试通过：`go test ./... -race -count=1`
- [ ] 代码风格检查：`gofmt -l -s .` 无输出
- [ ] 静态检查：`go vet ./...` 无错误
- [ ] 变更量 < 500 行（如有特殊原因请说明）
- [ ] 提交信息遵循 Conventional Commits
- [ ] 新功能有对应文档
- [ ] API 变更已更新 CHANGELOG.md

---

## 代码规范

### Go 代码风格

- 使用 `gofmt` 格式化代码
- 通过 `go vet` 静态检查
- 导出类型/函数/常量必须有中文注释（`// Xxx 实现...`）
- 错误处理优先，避免 `_` 忽略错误
- 优先使用标准库，避免引入新依赖

### 测试规范

- 测试文件命名：`xxx_test.go`
- 测试函数命名：`TestXxx` / `TestXxx_SubCase`
- 使用 `testing` 标准库，不引入第三方测试框架
- Mock 接口定义在测试文件中，优先使用小接口
- 测试必须通过竞态检测（`-race`）

### 文档规范

- 中文撰写，技术术语保留英文（API、HTTP、JSON 等）
- 代码示例使用可运行的 Go 代码片段
- 文件名和目录名使用英文小写加连字符

### 提交信息规范

```
<type>: <简短描述>

<可选详细说明>
```

示例：

```
feat: 添加上下文压缩模块

实现自动摘要策略，当会话窗口使用率超过 80% 时触发压缩。
保留最近 2 轮对话不被压缩。
```

---

## 开发环境

### 环境要求

- Go 1.26+
- Git

### 快速开始

```bash
# 克隆仓库
git clone https://github.com/wly2lcl/basework.git
cd basework

# 安装依赖
go mod download

# 运行测试
go test ./... -race -count=1

# 构建
make build

# 运行 CLI
./basework agent
```

### 常用命令

```bash
# 全量测试
go test ./... -count=1 -timeout=180s

# 竞态检测
go test ./... -race -count=1

# 覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 代码风格检查
gofmt -l -s .

# 静态检查
go vet ./...

# 构建（含可选模块）
go build -tags memory -o bin/basework-full ./cmd/basework

# 整理依赖
go mod tidy
```

### 可选模块

```bash
# 启用记忆模块构建
go build -tags memory ./cmd/basework

# 启用所有可选模块
go build -tags "memory tui" ./cmd/basework
```

---

## 代码审查

### 审查流程

1. 提交 PR 后，CI 自动运行测试和 lint
2. 至少需要 1 个审查者批准
3. 审查者提出修改意见 → 提交者更新 → 重新审查
4. 所有对话 resolved 后合并

### 审查标准

- **正确性**：代码逻辑正确，边界情况处理完善
- **测试覆盖**：新代码有测试，测试有意义
- **代码风格**：符合 Go 惯例，可读性好
- **性能**：避免不必要的内存分配和锁竞争
- **兼容性**：不破坏公共 API，废弃遵循策略

### PR 大小

- 保持小 PR（< 500 行变更）
- 大功能拆分为多个 PR
- 特殊情况（如自动生成代码）需在 PR 描述中说明

---

## 文档

### 文档要求

- 新功能必须有使用文档
- API 变更必须更新 `CHANGELOG.md`
- 文档用中文撰写，技术术语保留英文
- 代码示例必须可运行

### 文档位置

| 文件 | 内容 |
|------|------|
| `README.md` | 项目简介、快速开始、核心特性 |
| `ARCHITECTURE.md` | 架构概览、模块依赖、数据流 |
| `ROADMAP.md` | 路线图、已完成/规划中功能 |
| `CHANGELOG.md` | 版本变更记录 |
| `docs/DESIGN.md` | 详细设计文档、接口定义、设计决策 |
| `docs/TASKS.md` | 任务清单、实施阶段 |
| `docs/STATUS.md` | 项目状态、已完成/待完成功能 |
| `docs/guides/` | 使用指南（嵌入、配置、扩展） |

---

## 行为准则

### 我们的承诺

- 使用友好、包容的语言
- 尊重不同的观点和经验
- 优雅地接受建设性批评
- 关注对社区最有利的事情

### 我们的标准

**积极行为**：

- 使用欢迎和包容的语言
- 尊重不同观点和经验
- 优雅地接受建设性批评
- 关注对社区最有利的事情

**不可接受行为**：

- 性暗示语言或图像
- 挑衅、侮辱/贬低性评论
- 公开或私下的骚扰
- 未经明确许可发布他人隐私信息

---

## 常见问题

### 如何选择第一个贡献？

- 查看标有 `good first issue` 标签的 Issue
- 改进文档和测试也是很好的起点
- 阅读 `docs/TASKS.md` 了解各阶段任务

### 需要先讨论再实现吗？

- 小修复和文档改进可以直接提交 PR
- 新功能建议先创建 Issue 讨论设计方向
- 大变更（> 500 行）务必先讨论

### 如何联系维护者？

- 通过 GitHub Issues 讨论项目相关
- 在 PR 中 @ 相关维护者

---

## 许可证

贡献代码即表示你同意将其贡献至 MIT 许可证下。
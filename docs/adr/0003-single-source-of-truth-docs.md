# ADR 0003: 文档数字与结构必须机器生成，且单一权威

- Status: Accepted
- Amended by: [ADR 0004](0004-centralize-docs-and-task-tracking.md)（文档位置与任务进度）
- Date: 2026-09-12
- Supersedes: —

## 背景

文档层同时存在三类失真，且每一个都不是「错一次」，而是持续、静默地漂移：

1. **两份互斥的 ROADMAP** —— 根目录 `ROADMAP.md` 与 `docs/ROADMAP.md` 对 Phase 状态描述互相矛盾，读者无法判断哪份有效
2. **手写数字漂移约 45%** —— `docs/STATUS.md` 声称约 50,051 行代码、1,372 个测试；实测约 57,816 行（自动统计口径），1,645 个测试函数
3. **版本号三处冲突** —— git tag 为 `v0.1.2`，`CHANGELOG.md` 写到 `0.5.0`，文档写 `0.2.0`，且 `0.3.0/0.4.0/0.5.0` 并无对应 tag

根因是同一件事被人工维护了多份。此外，`ARCHITECTURE.md` 声称「`pkg/` 层不依赖任何第三方库」，而实际有 4 个（`golang.org/x/image`、`golang.org/x/oauth2`、`modernc.org/sqlite`、`github.com/golang/snappy`）——**结构性断言同样会漂移**，不只是数字。

考虑过的替代方案：靠「写文档时更仔细」来避免。否决理由是——人工维护的数字不会只错一次，而会随每次改动持续偏离，且没有任何机制在偏离时发出信号。

## 决定

**凡是机器能算的，不写进文档；凡是人工写的，只能有一份权威源。**

| 内容 | 权威源 | 生成方式 |
|---|---|---|
| 代码规模、测试数量 | `docs/STATS.md` | `make stats`（`scripts/docstats`） |
| 模块层级与依赖关系、第三方依赖边界 | `docs/DEPGRAPH.md` | `make deps`（`scripts/gendeps`） |
| 未来规划 | `docs/ROADMAP.md` | 人工，**唯一一份** |
| 当前状态 | `docs/STATUS.md` | 人工，只描述现状、不含规划 |
| 可执行清单 | `docs/TASKS.md` | 人工 |
| 变更记录 | `docs/CHANGELOG.md` | 人工；版本号**以 git tag 为权威** |

配套两条门禁，均在 CI 的 `quality` job：

- **结构门禁**：`tests/arch_test.go` 与 `scripts/gendeps` 校验层级与依赖边界
- **新鲜度门禁**：`make gen` 后执行 `git diff --exit-code docs/DEPGRAPH.md docs/STATS.md`，生成物与提交版本不一致即失败

生成器有两条硬性要求，否则门禁不可用：**不输出时间戳**、**所有列表字典序排序**。

## 后果

正面：
- 数字漂移从根上消失：文档中的数字只有一个来源，且 CI 校验生成物是否为最新
- 结构漂移同样被拦：`ARCHITECTURE.md` 那类「声称 pkg 无第三方依赖」的失实会被 `docs/DEPGRAPH.md` 直接暴露
- 读者不再需要判断「哪份文档是真的」——每个问题只有一个答案所在地

代价与后续：
- 改代码后需要运行 `make gen`，多一步操作（CI 会提醒，且本地可直接运行）
- 生成器自身必须保持确定性；一旦加入时间戳或未排序的输出，门禁会永久失败。这一点已写进两个生成器的文件头注释
- `make stats` 已固定 `-no-cover`；覆盖率用 `make coverage` 单独查看，不写入确定性生成物。
- 原设计分册已归档到 `docs/archive/design/`，当前设计入口为 `docs/DESIGN.md`。

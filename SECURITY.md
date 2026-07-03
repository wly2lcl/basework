# 安全策略 (Security Policy)

## 支持的版本 (Supported Versions)

| 版本 | 支持状态 |
|------|----------|
| 0.1.x | ✅ 积极支持 |
| < 0.1 | ❌ 不支持 |

只有最新次版本 (minor version) 会收到安全更新。请始终使用最新的稳定版本。

## 漏洞报告 (Reporting a Vulnerability)

我们非常重视安全问题。如果您发现安全漏洞，请通过以下方式报告：

### 首选方式：GitHub Issues

1. 前往 [GitHub Issues](https://github.com/basework/basework/issues)
2. 创建新的 Issue，标题以 `[SECURITY]` 开头
3. 详细描述漏洞内容和复现步骤
4. **请勿**在公开 Issue 中包含敏感信息（如凭证泄露）

### 备选方式：邮件

如果漏洞涉及敏感信息，请发送邮件至：

```
security@basework.dev
```

我们承诺：

- **48 小时内**确认收到报告
- **7 天内**给出初步评估和修复时间表
- 修复完成后，在更新日志 (CHANGELOG) 中致谢（除非您要求匿名）

## 安全更新策略 (Security Update Policy)

- 安全修复会作为补丁版本 (patch release) 发布，版本号格式为 `0.1.x`
- 每次安全更新会附带详细的变更日志说明
- 关键安全修复会在发布后 72 小时内同步更新至文档

## 已知安全限制 (Known Security Limitations)

Basework 作为一个 AI Agent 框架，在设计上存在以下安全考量：

### Bash 工具

- `pkg/tool/` 中的 bash 工具允许执行任意 shell 命令
- **风险**: 恶意提示可能诱导 Agent 执行危险命令
- **缓解措施**:
  - 建议在沙箱或容器环境中运行
  - 生产环境应限制 Agent 的系统权限
  - 后续版本将引入权限白名单机制 (Phase 14)

### LLM Provider API Key

- Provider API Key 通过配置文件或环境变量传入
- **风险**: 配置文件泄露可能导致 API Key 被盗用
- **缓解措施**:
  - 支持环境变量方式加载 (推荐)
  - 配置文件建议设置为 `600` 权限
  - 加入 `.gitignore` 避免误提交

### 会话数据持久化

- Session 数据以 JSONL 格式存储在本地文件系统
- **风险**: 敏感对话内容可能被未授权访问
- **缓解措施**:
  - 确保存储目录权限正确
  - 后续版本将引入数据加密存储

### MCP/LSP 集成

- MCP 和 LSP 客户端会与外部进程进行通信
- **风险**: 恶意 MCP/LSP 服务端可能执行危险操作
- **缓解措施**:
  - 仅连接可信的 MCP/LSP 服务端
  - MCP 工具调用受 Agent 工具循环管控

## 依赖安全策略 (Dependency Security Policy)

- 所有依赖通过 `go.mod` / `go.sum` 锁定版本，确保可重复构建
- 定期使用 `go vulnerability scan` 扫描已知漏洞
- 第三方依赖的安全更新会在评估后 2 周内合并
- 核心依赖 (Cobra、SQLite) 紧跟上游安全更新

## 安全最佳实践 (Security Best Practices)

1. **运行环境**: 在 Docker 容器或虚拟机中运行 Basework
2. **权限最小化**: 使用专用系统用户运行，限制文件系统写入范围
3. **网络隔离**: 除非必要，不要将 Agent 暴露在公网
4. **定期更新**: 始终使用最新版本
5. **审计日志**: 启用 Session 持久化以保留操作审计轨迹
6. **API Key 管理**: 使用密钥管理服务 (如 Vault) 而非硬编码
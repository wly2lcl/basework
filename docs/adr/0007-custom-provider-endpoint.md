# ADR 0007: 自定义模型端点的配置载体与解析优先级

- Status: Accepted
- Date: 2026-09-13
- Supersedes: —

## 背景

`provider.Config.BaseURL` 一直存在，但**只有可嵌入 API 能传**：`cmd/basework` 的
`newRuntimeAgent` 调用 `provider.Create` 时从不设置它（全仓库非测试代码中
`BaseURL:` 赋值 0 处），配置文件里也没有对应字段。

后果是**静默失效**，而且失败信息与真实原因完全无关：中转站、企业网关、自建兼容
服务这三类用户按 `docs/guides/cli-guide.md` 写 `providers.<name>.base_url` 后不会
收到任何配置错误，请求照旧打到 `api.openai.com`，最终以「鉴权失败 (401)」的形式
暴露——把「端点没生效」误诊为「key 不对」。这是 REL-003 拿到真实凭证做验证时
才发现的：把 `base_url` 指向不可达地址，收到的仍是 401 而非连接错误。

必须回答三个问题，否则每个调用点会自行发明答案：

1. **载体**：自定义端点写在配置文件、环境变量，还是两者都支持？
2. **优先级**：`BASEWORK_PROVIDER` 与 provider 级配置块、环境变量端点之间谁赢？
3. **如何避免再次静默失效**：写错（漏 scheme、写成相对路径）时必须报错，
   而不是悄悄退回默认端点。

考虑过的替代方案：**只支持环境变量**（如 `OPENAI_BASE_URL`）——被否决。多 provider
共用一个配置文件时，环境变量无法表达「哪个 provider 用哪个端点」，且端点与
provider 的对应关系会散落在进程环境里，`config explain` 也就无法复现有效值。
另一个方案是**顶层单个 `base_url` 字段**——同样被否决：它假定同时只有一个生效
provider，一旦 `BASEWORK_PROVIDER` 切换就会误用另一个端点的配置。

## 决定

我们决定采用「**配置文件 provider 级 `providers.<name>.base_url` 为主，环境变量
`BASEWORK_BASE_URL` 为一次性覆盖**」，并明确一条优先级链：

```
配置文件 providers.<生效 provider>.base_url  →  BASEWORK_BASE_URL（环境变量，覆盖前位）
```

- **载体**：`Config.Providers` 是 `map[string]ProviderEndpoint`，键为 provider
  名称，值为 `{ "base_url": ..., "api_key": ... }`。与既有的 provider 专属块
  （`ollama.endpoint`、`azure.*`、`bedrock.*`、`opencode.api_key`）并存，
  不替换它们。
- **生效 provider 的判定**：`BASEWORK_PROVIDER` 优先于配置文件的 `provider` 字段
  （沿用既有行为）。`providers.<name>` 的键**按生效 provider 取值**，因此
  `BASEWORK_PROVIDER=anthropic` 时会读 `providers.anthropic`，而不是配置里的
  `provider` 所指那一块。
- **优先级**：环境变量 `BASEWORK_BASE_URL` 晚于配置文件生效，后位覆盖前位
  （与 ADR 0006 一致）。它覆盖的是「生效 provider 的端点」，不改变 provider 本身。
- **同源冲突**：`ollama` 同时有旧的 `ollama.endpoint` 与新的 `providers.ollama.base_url`
  时，**以 provider 级 `base_url` 为准**，产品层不再向 provider 传旧的 endpoint，
  避免出现两个真值。
- **未配置时旧行为**：`Providers` 为空（默认）且 `BASEWORK_BASE_URL` 未设置时，
  `provider.Create` 收到的 `BaseURL` 为空，行为与本项引入前完全一致——旧的嵌入
  路径与旧配置不受影响。
- **不静默回落**：`base_url` 的形态在配置加载时校验（必须是绝对 `http`/`https`
  URL 且带主机名），环境变量来源在启动时校验。空值合法（表示用默认端点），
  但写了却写错一律报错，不允许退回默认端点。`openai-compat` 类型没有 base_url
  时由 `provider.Create` 直接报错（既有行为，保持）。
- **凭据不外泄**：端点 URL 不是秘密，`config explain` 会显示它以便排查；但若 URL
  内嵌 userinfo（`https://user:pass@host`），展示时去掉 userinfo。`api_key` 一律
  只在脱敏视图里出现占位符。
- **解析只有一份实现**：`Config.ResolveEndpoint(providerType, envBaseURL)` 是纯函数
  （环境值由调用方注入），runtime 与 `config explain` 共用它，结果必须一致。

## 后果

- 正面：中转站/网关/自建端点有唯一、可解释、可复现的配置路径；写错的端点在加载
  或启动阶段就失败，不会退化成莫名其妙的 401；`config explain` 与 runtime 用同一
  个解析函数，不会分叉。
- 正面：`docs/guides/cli-guide.md` 里那段此前**纯属虚构**的 schema 之所以能保留，
  是因为代码现在真的实现了它——文档与代码一致，而不是把文档改得和残破实现一样。
- 代价：`Config` 多了一个 map 字段，克隆时需要深拷贝；新增 provider 类型时若要
  支持自定义端点，必须同时确认该类型的构造器真的消费 `BaseURL`
  （`newOllama` 这类有专属 opts 的路径需要显式处理优先级）。
- 后续：若将来需要 per-provider 的环境变量别名（如 `OPENAI_BASE_URL`），
  必须在本 ADR 的优先级链里补一行，并同时更新 `config explain` 的展示。

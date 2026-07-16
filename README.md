# tiny-proxy

本地 HTTP 代理，让 Codex CLI / ChatGPT App 通过 DeepSeek 大模型运行。

**核心功能：** OpenAI Responses API ↔ DeepSeek Chat Completions 协议双向转换，SSE 流式桥接，thinking 模式完整支持，多轮对话 + 工具调用兼容。

## 架构

```
Codex / ChatGPT App ──(Responses API)──▶ tiny-proxy ──(Chat Completions)──▶ DeepSeek API
       127.0.0.1:3688                                  api.deepseek.com/v1
```

代理零认证 — 上游 API key 仅在服务端配置，Codex 调用无需验证。

## 快速开始

### 1. 安装

```bash
git clone https://github.com/terry/tiny-proxy.git
cd tiny-proxy
make build
```

### 2. 配置

```bash
export DEEPSEEK_API_KEY=sk-your-deepseek-key
```

### 3. 启动

```bash
./tiny-proxy
```

首次运行会自动：
- 备份 `~/.codex/config.toml` → `config.toml.bak`
- 修改 Codex 配置指向本地代理（`model_provider = "tiny-proxy"`）
- 更新 `~/.codex/auth.json`
- 启动代理服务在 `127.0.0.1:3688`

### 4. 使用

启动 Codex / ChatGPT App，正常对话即可。

## 命令参考

```bash
tiny-proxy                   # 启动代理（默认端口 3688）
tiny-proxy --setup           # 仅修改 Codex 配置，不启动代理
tiny-proxy --setup --dry-run # 预览将要做的配置修改
tiny-proxy --restore         # 恢复 Codex 原始配置
tiny-proxy --port 4000       # 使用自定义端口
```

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DEEPSEEK_API_KEY` | 是 | — | DeepSeek API 密钥 |
| `PROXY_PORT` | 否 | `3688` | 代理监听端口 |
| `DEEPSEEK_BASE_URL` | 否 | `https://api.deepseek.com/v1` | DeepSeek API 端点 |
| `DEEPSEEK_MODEL` | 否 | `deepseek-v4-flash` | 默认模型 |
| `REASONING_EFFORT` | 否 | `high` | 推理力度 |
| `STORE_TTL` | 否 | `3600` | 会话存储 TTL（秒） |
| `STORE_MAX` | 否 | `500` | 最大会话数 |
| `LOG_LEVEL` | 否 | `info` | 日志级别 |

## 端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/health` | — | 健康检查 |
| `GET` | `/v1/models` | — | 模型列表 |
| `POST` | `/v1/responses` | — | 主端点，协议转换 |
| `POST` | `/v1/chat/completions` | — | 直接透传（调试用） |

## 协议转换

### 请求：Responses API → Chat Completions

处理链：`ConvertRequest` → `injectHistory` → `EnsureThinkingSafety` → `NormalizeMessages`

| Responses 字段 | Chat 字段 | 说明 |
|---------------|-----------|------|
| `model` | `model` | 透传，缺省用配置值 |
| `input` (string) | `messages[{role:"user", content}]` | 字符串输入 → user 消息 |
| `input` (array) | `messages[]` | 数组按 item type 逐一转换 |
| `instructions` | `messages[{role:"system", content}]` | 系统指令 → system 消息 |
| `max_output_tokens` | `max_tokens` | 字段重命名 |
| `temperature` / `top_p` / `user` | 同名透传 | — |
| `stream` / `stream_options` | `stream` / `stream_options.include_usage` | 流式启用时附加 usage |
| `previous_response_id` | 触发历史合并 | 从 Store 取回消息并注入 |
| `reasoning.effort` | `reasoning_effort` 或 `thinking` | 见下方映射表 |
| `tools[]` | `tools[]` | 格式转换 + 类型过滤 |
| `tool_choice` | `tool_choice` | 仅在 tools 非空时写入 |
| `parallel_tool_calls` | `parallel_tool_calls` | 仅在 tools 非空时写入 |

#### input 数组 item 类型转换

| input type | 输出 role | 说明 |
|-----------|----------|------|
| `message` (role=user/assistant) | `user` / `assistant` | 含 content 块（input_text, output_text, text）合并 |
| `message` (role=developer) | `system` | developer 映射为 system |
| `message` (role=system) | `system` | 直接透传 |
| `function_call` | `assistant` + `tool_calls` | call_id, name, arguments → tool_calls[0] |
| `function_call_output` | `tool` | call_id → tool_call_id, output → content |

#### reasoning 映射

| Responses `reasoning.effort` | Chat 字段 |
|------------------------------|-----------|
| `none` | `thinking: {type: "disabled"}` |
| `auto` | `reasoning_effort: "auto"` |
| `minimal` | `reasoning_effort: "low"` |
| `low` / `medium` / `high` / `xhigh` | 直接透传 |

#### tools 转换

```
Responses 格式 (扁平):                Chat 格式 (function 包装):
{                                     {
  "type": "function",         →         "type": "function",
  "name": "...",                        "function": {
  "description": "...",                   "name": "...",
  "parameters": {...}                     "description": "...",
}                                         "parameters": {...}
                                        }
                                      }
```

- **`type: "function"`** → 转换为带 `function` 包装的 Chat 格式
- **`type: "custom"`** 等 → **过滤掉**（Chat API 不支持）
- 已经是 Chat 格式（带 `function` 包装）的 tools → 直接透传

#### 消息归一化（NormalizeMessages）

发送前对消息列表做最终修正：

1. **Tool 消息重排序** — tool 消息必须紧跟在对应 assistant tool_calls 之后，重新排列消息顺序
2. **孤儿 tool 降级** — 没有匹配 assistant 的 tool 消息降级为 user 消息（前缀 "Function call output"）

#### 历史注入（injectHistory）

当请求包含 `previous_response_id` 时：

1. 从 Store 取出上次存储的完整消息链 + reasoning
2. 与当前请求消息合并
3. 注入缓存的 `reasoning_content` 到最后一条 assistant 消息

**跳过条件**：input 中已含 `function_call` 项时跳过注入（避免 tool call 消息重复）

#### thinking 安全网（EnsureThinkingSafety）

发送前扫描所有 assistant 消息：
- 如果存在 assistant 带 `tool_calls` 但无 `reasoning_content` → 强制禁用 thinking
- 避免 DeepSeek 因缺少 reasoning_content 而拒绝请求

### 响应：Chat Completions → Responses API

#### 非流式（`handleNonStream`）

| Chat 字段 | Responses 字段 |
|-----------|---------------|
| `choices[0].message.content` | `output[{type:"message", content:[{type:"output_text", text}]}]` |
| `choices[0].message.tool_calls[]` | `output[{type:"function_call", call_id, name, arguments}]` |
| `choices[0].finish_reason` | `status`（`length` → `incomplete`, 其余 → `completed`） |
| `usage` | `usage`（prompt_tokens → input_tokens, completion_tokens → output_tokens） |
| 响应存储 | Store 保存消息链 + reasoning，供下一轮 `previous_response_id` 使用 |

#### 流式（`handleStream`）

SSE 状态机逐 chunk 转换，DevTools 兼容的 event 格式：

| 上游 SSE delta | 下游 SSE event |
|---------------|---------------|
| 首 chunk | `response.created` + `response.in_progress` |
| `delta.reasoning_content` | `response.reasoning_summary_text.delta` |
| `delta.content` | `response.output_text.delta` |
| `delta.tool_calls[].function.{name,arguments}` | `response.output_item.added` + `response.function_call_arguments.delta` |
| 流结束 | 关闭所有 output item → `response.completed` |

流完成后：
- 从 `GetAssistantMessage()` 提取 assistant 消息（含 `reasoning_content` + `content` + `tool_calls`）
- 与请求消息合并存入 Store

### reasoning 多轮缓存机制

DeepSeek thinking 模式要求多轮对话中每个 assistant 消息（特别是带 tool_calls 的）必须保留 `reasoning_content`：

1. **流式收集** — `StreamState` 从 delta 累加 `reasoning_content`
2. **存储注入** — `GetAssistantMessage()` 将 `reasoning_content` 写入 assistant 消息
3. **历史回放** — 下一轮请求时，`injectHistory` 从 Store 取出含 reasoning 的消息合并
4. **安全网** — `EnsureThinkingSafety` 兜底检查，防止任何遗漏导致 DeepSeek 400

## 项目结构

```
tiny-proxy/
├── main.go                   # CLI 入口，配置加载，Codex 配置管理
├── Makefile
├── config/
│   ├── env.go                # 环境变量加载 + 默认值
│   └── codex_toml.go         # Codex config.toml 读写 + auth.json 管理
├── convert/
│   ├── request.go            # Responses → Chat 请求转换（input, tools, reasoning）
│   ├── response.go           # Chat → Responses 非流式响应转换
│   ├── stream.go             # SSE 流式状态机（delta → Responses events）
│   ├── think.go              # reasoning 注入 + thinking 安全网
│   ├── normalize.go          # 消息归一化（tool 重排序 + 孤儿降级）
│   └── *_test.go             # 各模块单元测试
├── session/
│   └── store.go              # LRU + TTL 会话存储
├── upstream/
│   └── deepseek.go           # DeepSeek Chat Completions HTTP 客户端
├── proxy/
│   ├── server.go             # HTTP 路由注册
│   ├── handler_responses.go  # 核心处理链（转换 → 注入 → 安全网 → 归一化 → 发送）
│   ├── handler_models.go     # /v1/models
│   ├── handler_health.go     # /health
│   └── handlers_test.go      # handler 测试
├── scripts/
│   └── smoke.sh              # 端到端冒烟测试
└── docs/
    └── superpowers/
        ├── specs/            # 设计文档
        └── plans/            # 实现计划
```

## 测试

```bash
go test ./... -v -count=1    # 全部单元测试
./scripts/smoke.sh           # 端到端冒烟测试（需先启动代理）
```

## 不支持的类型（已知限制）

| 类型 | 原因 |
|------|------|
| `input_image` / `image_url` 内容块 | 未实现，图片输入暂不支持 |
| `type: "custom"` 工具（web_search 等） | Chat API 不支持，过滤丢弃（未做 function proxy） |
| tool parameters schema 自动修复 | 未实现，依赖上游 schema 完整性 |
| 按 tool_call_id 粒度的 reasoning 缓存 | 当前按 response 粒度缓存，多 tool call 轮次可能不够精确 |

## 技术栈

- **Go 1.26** / 标准库 `net/http`
- **gjson + sjson** — 零反序列化 JSON 操作
- **无其他运行时依赖** — 单二进制部署（~9 MB）

## 参考

- [codex-bridge](https://github.com/wujfeng712-ui/codex-bridge) — Node.js 实现，协议转换逻辑参考
- [ccx](https://github.com/BenedictKing/ccx) — Go 实现，消息归一化 + Codex tool compat 参考

## License

MIT

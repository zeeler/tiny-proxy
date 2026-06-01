# tiny-proxy 设计文档

## 概述

tiny-proxy 是一个用 Go 实现的本地 HTTP 代理服务，专门为 Codex CLI 调用 DeepSeek 大模型设计。核心功能是将 Codex 的 OpenAI Responses API 协议转换为 DeepSeek 的 Chat Completions API 协议，并自动管理 Codex 的 config.toml 配置。

**设计原则：**
- 单二进制、零外部依赖（除两个轻量 JSON 库）
- DeepSeek 专用，不做多供应商路由
- 无 Web 界面，纯 CLI
- 参考 codex-bridge 的协议转换逻辑，用 Go 重写

---

## 一、整体架构

```
┌──────────────┐   Responses API    ┌──────────────────┐   Chat Completions   ┌──────────────┐
│  Codex CLI   │───────────────────▶│  tiny-proxy (Go) │─────────────────────▶│ DeepSeek API │
│              │  Bearer <key>      │   127.0.0.1:3688  │  Bearer <deepseek>   │              │
└──────────────┘                    └──────────────────┘                      └──────────────┘
                                            │
                                            │ 启动时自动修改
                                            ▼
                                   ~/.codex/config.toml
```

### 技术选型

| 维度 | 选择 | 理由 |
|------|------|------|
| 语言 | Go 1.26+ | 编译为单二进制，性能好，部署简单 |
| HTTP 框架 | `net/http` 标准库 | 零外部依赖 |
| JSON 处理 | `github.com/tidwall/gjson` + `github.com/tidwall/sjson` | 零反序列化，高性能，轻量 |
| TOML 处理 | `github.com/BurntSushi/toml` | Go 生态 TOML 标准库 |
| 配置方式 | 环境变量 + CLI 参数 | 简洁，参考 codex-bridge |

---

## 二、协议转换

### 2.1 请求转换：Responses → Chat Completions

```
POST /v1/responses                           POST /v1/chat/completions
{                                            {
  "model": "deepseek-v4-flash",      ──▶       "model": "deepseek-v4-flash",
  "input": "hello",                            "messages": [
  "instructions": "...",                         {"role":"system","content":"..."},
  "tools": [...],                                {"role":"user","content":"hello"}
  "reasoning": {"effort":"high"},              ],
  "previous_response_id": "resp_xxx",          "tools": [...],
  "stream": true                               "reasoning_effort": "high",
}                                              "stream": true
                                             }
```

**字段映射表：**

| Responses 字段 | Chat 字段 | 处理逻辑 |
|---|---|---|
| `model` | `model` | 直接透传 |
| `input` (string) | `messages[]` | 包装为单条 `role:user` 消息 |
| `input` (array) | `messages[]` | 逐条转换，按 type 分发 |
| `instructions` | `messages[0]` (system) | 插入消息数组头部 |
| `previous_response_id` | 历史消息拼接 | 从 LRU 存储查找前序对话，拼到 messages 前面 |
| `tools[]` | `tools[]` | 格式兼容，直接透传 |
| `reasoning.effort` | `reasoning_effort` | 枚举值映射 |
| `max_output_tokens` | `max_tokens` | 字段重命名 |
| `temperature` | `temperature` | 直接透传 |
| `top_p` | `top_p` | 直接透传 |
| `tool_choice` | `tool_choice` | 仅当 tools 存在时传递 |
| `parallel_tool_calls` | `parallel_tool_calls` | 仅当 tools 存在时传递 |

**input 数组消息类型分发：**

| type | 转换结果 |
|---|---|
| `message` | `messages[]` 条目，content 支持 text/image 块 |
| `function_call` | `role:"assistant"` + `tool_calls[]` |
| `function_call_output` | `role:"tool"` + `tool_call_id` + `content` |

**推理力度映射（DeepSeek 特定）：**

| Codex 值 | DeepSeek 值 |
|---|---|
| `none` | `thinking: {type: "disabled"}` |
| `minimal` | `reasoning_effort: "low"` |
| `low` / `medium` / `high` / `xhigh` | 直接透传 |

### 2.2 响应转换：Chat → Responses（非流式）

将 Chat Completions 的 `choices[0].message` 转换为 Responses API 的 `output[]` 数组：

- `message.content` → `output[].content[]` 中的 `output_text` 块
- `message.tool_calls[]` → `output[]` 中的 `function_call` 项
- `message.reasoning_content` → `output[]` 中的 `reasoning` 项
- `finish_reason` → `status`（`stop` → `completed`，`length` → `incomplete`，`tool_calls` 保持不变）
- `usage` 对象做字段重命名（`prompt_tokens` → `input_tokens`，`completion_tokens` → `output_tokens`）

### 2.3 流式 SSE 桥接

核心是一个状态机，逐 chunk 做实时转换：

```
Chat SSE 事件                         Responses SSE 事件
─────────────────────────────────────────────────────────────
收到首个 chunk                   →  event: response.created
                                     event: response.in_progress

delta.reasoning_content          →  event: response.output_item.added
                                     (type: reasoning)
                                     event: response.reasoning_summary_part.added
                                     event: response.reasoning_summary_text.delta

delta.content (普通文本)          →  event: response.output_item.added
                                     (type: message)
                                     event: response.content_part.added
                                     event: response.output_text.delta

delta.tool_calls[].function      →  event: response.output_item.added
                                     (type: function_call)
                                     event: response.function_call_arguments.delta

finish_reason                    →  关闭所有打开的 block

[DONE]                           →  event: response.completed
```

**状态机需要跟踪：**
- 当前是否在 reasoning/text/function_call block 中
- 每个 function_call index 的累积参数
- 事件序号（seq），每个 SSE 事件递增
- 已发射的 output_item 索引和 ID

### 2.4 Thinking 多轮回放

DeepSeek 的 `reasoning_content` 仅在生成时返回，后续请求不会自动包含。

**处理方式：**
1. 首轮响应中提取 `reasoning_content`，按 response_id 缓存
2. 当 Codex 发送 `previous_response_id` 时，从缓存取出
3. 构造 assistant 消息时，将推理内容作为 `reasoning_content` 字段注入

### 2.5 会话管理（previous_response_id）

LRU 内存存储（默认 500 条，TTL 1 小时），key 为 response_id，value 为完整的 messages 历史 + reasoning_content。收到 `previous_response_id` 时从存储拼接上下文。

---

## 三、Codex 配置自动管理

### 3.1 管理的配置项

```toml
model = "deepseek-v4-flash"
model_provider = "tiny-proxy"
model_reasoning_effort = "high"

[model_providers.tiny-proxy]
name = "tiny-proxy"
base_url = "http://127.0.0.1:3688/v1"
wire_api = "responses"
requires_openai_auth = true
experimental_bearer_token = "PROXY_MANAGED"
```

### 3.2 管理流程

1. 启动时读取 `~/.codex/config.toml`
2. 检查是否已有 `[model_providers.tiny-proxy]` 节，有则仅更新 base_url，无则追加
3. 检查 `model` / `model_provider` 是否指向 tiny-proxy，否则写入
4. 修改前自动备份为 `config.toml.bak`
5. 同时更新 `~/.codex/auth.json` 中的 `OPENAI_API_KEY`

### 3.3 CLI 命令

```bash
tiny-proxy                        # 启动代理（默认端口 3688）
tiny-proxy setup                  # 仅修改配置，不启动代理
tiny-proxy setup --dry-run        # 预览将要做的修改
tiny-proxy restore                # 恢复原始配置
tiny-proxy --port 3699            # 自定义端口
```

---

## 四、环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `DEEPSEEK_API_KEY` | 是 | - | DeepSeek API 密钥 |
| `PROXY_PORT` | 否 | `3688` | 代理监听端口 |
| `PROXY_AUTH_KEY` | 否 | 自动生成 | 代理入站认证密钥 |
| `DEEPSEEK_BASE_URL` | 否 | `https://api.deepseek.com/v1` | DeepSeek 端点 |
| `DEEPSEEK_MODEL` | 否 | `deepseek-v4-flash` | 默认模型名 |
| `REASONING_EFFORT` | 否 | `high` | 默认推理力度 |
| `STORE_TTL` | 否 | `3600` | 会话 TTL（秒） |
| `STORE_MAX` | 否 | `500` | 最大会话数 |
| `LOG_LEVEL` | 否 | `info` | 日志级别 |

---

## 五、项目结构

```
tiny-proxy/
├── main.go                       # 入口，CLI 参数解析
├── go.mod / go.sum
├── config/
│   ├── env.go                    # 环境变量加载
│   └── codex_toml.go             # config.toml 读写
├── proxy/
│   ├── server.go                 # HTTP 服务器，路由分发
│   ├── handler_responses.go      # POST /v1/responses
│   ├── handler_models.go         # GET /v1/models
│   └── handler_health.go         # GET /health
├── convert/
│   ├── request.go                # Responses → Chat 请求转换
│   ├── response.go               # Chat → Responses 响应转换（非流式）
│   ├── stream.go                 # SSE 流式桥接（状态机）
│   └── think.go                  # Thinking 缓存与回放
├── session/
│   └── store.go                  # LRU 会话存储
├── upstream/
│   └── deepseek.go               # DeepSeek HTTP 客户端
└── Makefile
```

---

## 六、端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/health` | 否 | 健康检查 |
| `GET` | `/v1/models` | 是 | DeepSeek 模型列表 |
| `POST` | `/v1/responses` | 是 | **主端点**，协议转换 |
| `POST` | `/v1/chat/completions` | 是 | 透传（调试用） |

---

## 七、错误处理

| 场景 | HTTP 状态码 | 说明 |
|------|-------------|------|
| 上游不可达 | 502 | 返回标准错误 JSON |
| 上游超时（120s） | 504 | 超时错误 |
| 认证失败 | 401 | 密钥不匹配 |
| 模型不支持 | 400 | 仅支持 deepseek 前缀模型 |
| 上游返回错误 | 透传状态码 | 保留上游错误信息 |

---

## 八、依赖

```
github.com/tidwall/gjson      # JSON 读取（零反序列化）
github.com/tidwall/sjson      # JSON 写入
github.com/BurntSushi/toml    # TOML 解析
```

仅 3 个外部依赖，均来自 Go 生态的成熟轻量库。

# tiny-proxy

本地 HTTP 代理，让 [Codex CLI](https://github.com/openai/codex) 通过 DeepSeek 大模型运行。

**核心功能：** 将 Codex 的 OpenAI Responses API 协议转换为 DeepSeek 的 Chat Completions 协议，双向转换 + SSE 流式桥接 + thinking 模式支持 + 自动管理 Codex 配置。

## 工作原理

```
Codex CLI ──(Responses API)──▶ tiny-proxy ──(Chat Completions)──▶ DeepSeek API
                 :3688                              api.deepseek.com
```

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
- 修改 Codex 配置指向本地代理
- 更新 `~/.codex/auth.json` 认证信息
- 启动代理服务在 `127.0.0.1:3688`

### 4. 使用

启动 Codex，正常对话即可。所有请求会通过 tiny-proxy 转发到 DeepSeek。

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
| `PROXY_AUTH_KEY` | 否 | 自动生成 | 代理入站认证密钥 |
| `DEEPSEEK_BASE_URL` | 否 | `https://api.deepseek.com/v1` | DeepSeek API 端点 |
| `DEEPSEEK_MODEL` | 否 | `deepseek-v4-flash` | 默认模型 |
| `REASONING_EFFORT` | 否 | `high` | 推理力度 |
| `STORE_TTL` | 否 | `3600` | 会话存储 TTL（秒） |
| `STORE_MAX` | 否 | `500` | 最大会话数 |
| `LOG_LEVEL` | 否 | `info` | 日志级别 |

## 端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| `GET` | `/health` | 否 | 健康检查 |
| `GET` | `/v1/models` | 是 | 模型列表 |
| `POST` | `/v1/responses` | 是 | 主端点，协议转换 |
| `POST` | `/v1/chat/completions` | 是 | 直接透传（调试用） |

## 协议转换覆盖

### 请求方向（Responses → Chat Completions）

- `input` (string/array) → `messages[]`
- `instructions` → system 消息
- `previous_response_id` → 历史消息拼接
- `reasoning.effort` → `reasoning_effort` / `thinking` 映射
- `max_output_tokens` → `max_tokens`
- `tools[]` / `tool_choice` / `parallel_tool_calls` 透传
- `function_call` / `function_call_output` → assistant tool_calls / tool 消息

### 推理力度映射

| Codex | DeepSeek |
|-------|----------|
| `none` | `thinking: {type: "disabled"}` |
| `minimal` | `reasoning_effort: "low"` |
| `low` / `medium` / `high` / `xhigh` | 直接透传 |

### 响应方向（Chat → Responses API）

- 非流式：`choices[0].message` → `output[]` 数组
- 流式：SSE 状态机实时转换，逐 chunk 桥接
- `reasoning_content` → reasoning output item（带多轮回放缓存）
- `tool_calls` → function_call output items
- `finish_reason` → `status` 映射（`length` → `incomplete`）

## 项目结构

```
tiny-proxy/
├── main.go                  # CLI 入口
├── Makefile
├── config/
│   ├── env.go               # 环境变量加载
│   └── codex_toml.go        # Codex 配置管理
├── convert/
│   ├── request.go           # Responses → Chat 请求转换
│   ├── response.go          # Chat → Responses 响应转换
│   ├── stream.go            # SSE 流式状态机
│   └── think.go             # Thinking 缓存注入
├── session/
│   └── store.go             # LRU+TTL 会话存储
├── upstream/
│   └── deepseek.go          # DeepSeek HTTP 客户端
├── proxy/
│   ├── server.go            # HTTP 路由 + 认证
│   ├── handler_responses.go # 核心协议转换处理
│   ├── handler_models.go    # 模型列表
│   └── handler_health.go    # 健康检查
├── scripts/
│   └── smoke.sh             # 端到端冒烟测试
└── docs/
    └── superpowers/
        ├── specs/           # 设计文档
        └── plans/           # 实现计划
```

## 测试

```bash
go test ./... -v -count=1    # 全部单元测试
./scripts/smoke.sh           # 端到端冒烟测试（需先启动代理）
```

## 技术栈

- **Go 1.26** / 标准库 `net/http`
- **gjson + sjson** — 零反序列化 JSON 操作
- **无其他运行时依赖** — 单二进制部署（~9 MB）

## 参考

- [codex-bridge](https://github.com/wujfeng712-ui/codex-bridge) — 协议转换逻辑参考
- [ccx](https://github.com/BenedictKing/ccx) — Go 实现参考

## License

MIT

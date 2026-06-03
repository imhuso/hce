<div align="right"><a href="./README.md">English</a> | <b>简体中文</b></div>

<h1 align="center">HCE · 代码语义检索引擎</h1>

<p align="center">用自然语言搜索你的代码库。客户端在本地扫描并推送变更，服务端负责切分、向量化与混合检索。</p>

<p align="center">
  <a href="https://github.com/imhuso/hce/actions/workflows/ci.yml"><img src="https://github.com/imhuso/hce/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/imhuso/hce/releases"><img src="https://img.shields.io/github/v/release/imhuso/hce?include_prereleases&sort=semver&color=blue" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/imhuso/hce" alt="Go version">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License: MIT"></a>
</p>

HCE（Hybrid Code Engine）是一个面向多代码库的语义检索引擎，采用 **client-server + push 模式**：

- **客户端（`hce`）** 在本地扫描代码库、做增量 diff，只把**变更文件的内容**推送到服务端。源码始终留在客户端，服务端从不读你的文件系统。
- **服务端（`hce-server`）** 接收文件内容，负责 AST 切分（tree-sitter）、内容去重、向量化（embedding）、写入向量库（Milvus）和混合检索。单个 HTTP 服务面向多个代码库，按 `codebase_id` 隔离到不同的 Milvus collection。

配套一个 Web 前端（`hce-web`），可在浏览器里跨代码库做自然语言检索。

---

## ✨ 核心特性

- **混合检索** — dense 向量召回 + keyword 倒排，双命中加 TF boost，单 keyword 命中作补充召回，兼顾语义相关性与精确匹配。
- **增量索引** — chunk 级 content-hash 去重：只有内容真正变化的代码块才会重新 embedding，大幅降低向量化成本。
- **快慢双路 diff** — 先用 size+mtime 快速判定未变文件（大库秒过），仅对疑似变更文件做 sha256 内容校验。
- **多语言 AST 切分** — 基于 tree-sitter，支持 16 种语言，按函数/类等语法边界切分，小文件整体成块、大节点递归、按行回退。
- **可插拔 Embedding** — 内置 OpenAI、Gemini、VoyageAI、Ollama，也兼容 LM Studio / vLLM 等 OpenAI 协议的自托管模型。
- **多代码库隔离** — 一个服务端按 `codebase_id` 服务任意多个项目，互不干扰。
- **推送架构** — 服务端无需访问你的代码目录，适合把索引服务集中部署、源码留在各开发机。
- **容错续传** — 单 batch 失败不中断 sync，state 每批落盘，中断可续传。

---

## 🚀 快速开始

### 方式一 —— docker compose 一键起全栈（推荐）

一条命令 —— 缺 `.env` 自动建、拉起全栈、等后端健康、再打印下一步：

```bash
make up
```

或手动逐步来：

```bash
# 1. 填入 embedding API key
cp .env.example .env
# 编辑 .env，至少填 HCE_EMBEDDING_API_KEY

# 2. 拉起 etcd + minio + milvus + hce-server + hce-web
docker compose up -d --build

# 3. 浏览器访问前端
open http://localhost:9528
```

> **没有 API key？** 可零密钥纯本地试用：装 [Ollama](https://ollama.com)、`ollama pull nomic-embed-text`，然后在 `.env` 里设 `HCE_EMBEDDING_PROVIDER=ollama`、`HCE_EMBEDDING_MODEL=nomic-embed-text`、`HCE_EMBEDDING_BASE_URL=http://host.docker.internal:11434`（见 `.env.example` 底部）。源码全程不出本机。

> 全栈本身只给你服务端 + Web UI —— 在装好 `hce` CLI（见下）并于项目里 `hce sync` 之前，UI 是空的。

### 方式二 —— 从源码起服务端

前置条件：Go 1.25+、已开启 **CGO**（tree-sitter 是 cgo 绑定）、Milvus 已运行在 `localhost:19530`。

```bash
export HCE_EMBEDDING_API_KEY=<your-key>
go run ./cmd/server -config configs/config.yaml
```

---

## 📦 安装 CLI

`hce` 是纯 Go 客户端（**无需 CGO**），跨平台单文件。三选一：

```bash
# A. go install（有 Go 环境；装到 ~/go/bin）
go install github.com/imhuso/hce/cmd/hce@latest

# B. 从 Releases 下载预编译二进制（无需 Go）—— darwin/linux/windows × amd64/arm64
#    https://github.com/imhuso/hce/releases —— 下载、解压、放进 PATH。
gh release download --repo imhuso/hce --pattern '*linux_amd64*'

# C. 从源码编译
go build -o hce ./cmd/hce && sudo mv hce /usr/local/bin/

hce version    # 发布版显示 tag 版本号，源码编译显示 dev
```

配一次机器级服务端地址（所有项目通用）：

```bash
hce config --base-url http://<服务器IP或域名>:9528/api/v1
```

---

## 🖥️ CLI 用法

```bash
hce sync                       # 扫描并把变更推送到服务端
hce search <query> [-k 10] [-f text|json] [--no-sync]   # 语义搜索（默认先 sync）
hce status                     # 当前 codebase 配置 / 上次 sync
hce list                       # 列出服务端所有已索引 collection
hce clear                      # 清除当前 codebase 的服务端索引 + 本地 state
hce init [--id <name>]         # 显式初始化 .hce/config.json
hce config [--base-url <url>]  # 查看 / 设置全局服务端地址（~/.hce/config.json）
hce version                    # 显示版本
```

| 选项 | 默认 | 说明 |
|------|------|------|
| `-p <path>` | 自动查找 | 项目根（从当前目录向上找 `.hce` 或 `.git`） |
| `--base-url <url>` | `http://localhost:9528/api/v1` | 服务端地址。优先级：旗标 > `HCE_BASE_URL` > 项目 `.hce/config.json` > 全局 `~/.hce/config.json` > 内置默认 |
| `-k <int>` | 10 | search 的 top_k |
| `-f text\|json` | text | search 输出格式 |
| `--no-sync` | — | search 时跳过 sync，仅检索 |

---

## ⚙️ 配置

服务端配置在 `configs/config.yaml`，**敏感信息一律通过环境变量注入，不要写进 yaml**（会进 git）。

### Embedding 供应商

通过 `HCE_EMBEDDING_PROVIDER` 选择，工厂在 `cmd/server/main.go` 的 `initEmbedding`：

| Provider | 说明 |
|----------|------|
| `gemini` | Google Gemini（默认，`gemini-embedding-001`） |
| `openai` | OpenAI，或任意 OpenAI 协议兼容端点（LM Studio / vLLM） |
| `voyageai` | VoyageAI |
| `ollama` | 本地 Ollama |

### 选哪个模型 & 免费额度

| 场景 | 推荐 | 理由 | 免费额度 |
|------|------|------|----------|
| **代码检索（首选）** | VoyageAI `voyage-code-3` | 专为代码训练，代码搜索准确率最高；32k 上下文，1024 维 | 每账号 **200M tokens 免费**，之后 $0.18 / 1M |
| **零成本、免信用卡** | Gemini `gemini-embedding-001` | 通用能力强，经 Google AI Studio 免卡即用 | 约 100 RPM / 每天 1,000 次请求 |
| **完全本地 / 离线** | Ollama 或 LM Studio（`nomic-embed-text`、`Qwen3-Embedding`） | 无 API 费用，源码不出本机 | 无限（取决于你的硬件） |
| **已有 OpenAI** | OpenAI `text-embedding-3-small` / `-large` | 有 key 时最省事 | 无免费档（$0.02 / $0.13 每 1M） |

本仓库默认 provider 为 `gemini-embedding-001`，便于零成本上手；**认真做代码检索建议换 `voyage-code-3`**，效果明显更好，其 200M 免费 token 足够把一个大型代码库索引很多遍（再加 content-hash 去重，重复 sync 几乎不耗额度）。

> 免费额度经常变动（Gemini 在 2025 年底下调过配额）。依赖前请到 [Voyage 定价](https://docs.voyageai.com/docs/pricing) 与 [Gemini 速率限制](https://ai.google.dev/gemini-api/docs/rate-limits) 核对当前数值。在不同维度的模型间切换（如 voyage `1024` ↔ gemini `3072`）需要 `clear` + 重建索引。

### 关键环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `HCE_EMBEDDING_API_KEY` | ✅ | 选定供应商的 API Key（`GEMINI_API_KEY` 亦可） |
| `HCE_EMBEDDING_PROVIDER` | | `gemini` / `openai` / `voyageai` / `ollama` |
| `HCE_EMBEDDING_MODEL` | | embedding 模型名 |
| `HCE_EMBEDDING_BASE_URL` | | 自托管 / 代理端点（容器内访问宿主用 `http://host.docker.internal:1234/v1`） |
| `HCE_EMBEDDING_DIM` | | 自托管非 OpenAI 模型需手动指定维度（如 Qwen3-Embedding-8B=4096） |
| `MILVUS_ADDRESS` | | Milvus 地址，默认 `localhost:19530` |

---

## 🏗️ 工作原理

```
┌─────────────────────────┐         push（仅变更文件内容）        ┌──────────────────────────────┐
│        hce          │  ───────────────────────────────▶   │          hce-server          │
│  （本地，持有源码）       │   POST /index/upsert  /delete       │  切分 → 去重 → embedding →    │
│                         │   POST /index/flush                 │  写入 Milvus → 混合检索        │
│  scan → diff → 批量推送  │  ◀───────────────────────────────   │                              │
└─────────────────────────┘         search 结果                  └───────────────┬──────────────┘
                                                                                 │
                                                              ┌──────────────────┼──────────────────┐
                                                              ▼                  ▼                  ▼
                                                         Embedding API       Milvus（向量库）    keyword 倒排
```

**sync（客户端 → 服务端）**

1. **Scan** 遍历项目根：扩展名白名单 + `.gitignore` + 内置忽略规则，跳过 >1MB / 二进制 / 非 UTF-8 文件。
2. **diff** 对比本地 `.hce/index.json`：快路径靠 size+mtime 判定未变；慢路径才算 sha256 验证内容。
3. 变更文件按 batch（默认 50 文件 / 5 MiB）并发 `POST /index/upsert`；删除文件走 `/index/delete`。
4. 服务端做 **chunk 级增量**：切分 → 算 content sha256 → 查 Milvus 已有 chunk → 命中 hash 的复用（跳过 embedding），新 chunk 批量 embed + insert，消失的旧 chunk delete。
5. 全部 batch 完成后客户端触发**一次** `/index/flush`，避免 Milvus compaction storm。

**search（混合检索）**

1. dense 向量召回 `top_k × 3`（多召回给 keyword 命中留空间）。
2. keyword 倒排命中 query 中的字面 token。
3. **merge**：向量 + keyword 双命中加 TF boost（cap 0.20）；仅 keyword 命中以较低基础分（cap 0.50）作补充召回。
4. 按 final score 排序取 `top_k`。

### 端口拓扑

| 组件 | 端口 | 说明 |
|------|------|------|
| `hce-server` | **9527** | 后端 HTTP API（`config.yaml` 的 `server.port`） |
| `hce-web`（nginx） | **9528** → 80 | 前端 + 反代：`/api/` → `hce-server:9527` |
| Milvus | 19530 | 向量库 |

> CLI 默认走 `http://localhost:9528/api/v1`（经 nginx 反代）。直连后端调试用 `--base-url http://localhost:9527/api/v1`。

### HTTP API

所有端点位于 `/api/v1`，统一返回 `{code, message, data}`。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/index/upsert` | 推送文件内容并增量索引 |
| POST | `/index/delete` | 删除指定文件的索引 |
| POST | `/index/flush` | 刷盘（一批 sync 完成后调用一次） |
| DELETE | `/index` | 清除整个 codebase |
| GET | `/indexes` | 列出所有已索引 collection |
| POST | `/search` | 语义搜索 |
| GET | `/health` | 健康检查 |

### 支持的语言

`Go` · `Python` · `JavaScript` · `TypeScript` · `Java` · `C` · `C++` · `C#` · `Ruby` · `Rust` · `PHP` · `Kotlin` · `Scala` · `Swift` · `Lua` · `Bash`

---

## 🛠️ 开发

```bash
# 编译服务端（CGO_CFLAGS 抑制上游 lua parser 的 NUL 字面量警告）
CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

# 编译 CLI
go build -o bin/hce ./cmd/hce/

# 静态检查（仓库暂无单元测试）
go vet ./...
go build ./...
```

更深的实现细节与坑（Milvus VARCHAR 上限、UTF-8 sanitize、embedding 非对称 task type、codebase_id → collection 派生方案）见 [`CLAUDE.md`](./CLAUDE.md)。

**发布**（maintainer）：版本号锚定在根 `package.json`。`npm run release`（[bumpp](https://github.com/antfu-collective/bumpp)）一键 bump + 打 tag + 推送；推送 `v*` tag 触发 `.github/workflows/release.yml`，交叉编译 hce 五平台并自动建 GitHub Release。

---

## 🤝 贡献

欢迎提 Issue 和 PR。CI 会在每次 push/PR 跑 `go vet` + `go build`，请先在本地确保两者通过。现有代码以中文注释为主，**中文或英文贡献都可以**。

## 📄 许可证

[MIT](./LICENSE) © imhuso

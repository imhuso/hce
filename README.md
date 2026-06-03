# HCE · 代码语义检索引擎

> 用自然语言搜索你的代码库。客户端在本地扫描并推送变更，服务端负责切分、向量化与混合检索。

HCE（Hybrid Code Engine）是一个面向多代码库的语义检索引擎，采用 **client-server + push 模式**：

- **客户端（`hce-cli`）** 在本地扫描代码库、做增量 diff，只把**变更文件的内容**推送到服务端。源码始终留在客户端，服务端从不读你的文件系统。
- **服务端（`hce-server`）** 接收文件内容，负责 AST 切分（tree-sitter）、内容去重、向量化（embedding）、写入向量库（Milvus）和混合检索。单个 HTTP 服务面向多个代码库，按 `codebase_id` 隔离到不同的 Milvus collection。

配套一个 Web 前端（`hce-web`），可在浏览器里跨代码库做自然语言检索。

---

## ✨ 核心特性

- **混合检索** — dense 向量召回 + keyword 倒排，双命中加 TF boost，单 keyword 命中作补充召回，兼顾语义相关性与精确匹配。
- **增量索引** — chunk 级 content-hash 去重：只有内容真正变化的代码块才会重新 embedding，大幅降低向量化成本。
- **快慢双路 diff** — 客户端先用 size+mtime 快速判定未变文件（大库秒过），仅对疑似变更文件做 sha256 内容校验。
- **多语言 AST 切分** — 基于 tree-sitter，支持 16 种语言，按函数/类等语法边界切分，小文件整体成块、大节点递归、按行回退。
- **可插拔 Embedding** — 内置 OpenAI、Gemini、VoyageAI、Ollama 四种供应商，也兼容 LM Studio / vLLM 等 OpenAI 协议的自托管模型。
- **多代码库隔离** — 一个服务端按 `codebase_id` 服务任意多个项目，互不干扰。
- **推送架构** — 服务端无需访问你的代码目录，适合把索引服务集中部署、源码留在各开发机。
- **容错续传** — 单 batch 失败不中断 sync，state 每批落盘，中断可续传。
- **一键全栈** — `docker compose` 拉起 etcd + minio + milvus + 后端 + 前端。

---

## 🏗️ 架构与数据流

```
┌─────────────────────────┐         push（仅变更文件内容）        ┌──────────────────────────────┐
│        hce-cli          │  ───────────────────────────────▶   │          hce-server          │
│  （本地，持有源码）       │   POST /index/upsert  /delete       │  切分 → 去重 → embedding →    │
│                         │   POST /index/flush                 │  写入 Milvus → 混合检索        │
│  scan → diff → 批量推送  │  ◀───────────────────────────────   │                              │
└─────────────────────────┘         search 结果                  └───────────────┬──────────────┘
                                                                                 │
                                                              ┌──────────────────┼──────────────────┐
                                                              ▼                  ▼                  ▼
                                                         Embedding API       Milvus（向量库）    keyword 倒排
```

### sync（客户端 → 服务端）

1. **Scan** 遍历项目根：默认扩展名白名单 + `.gitignore` + 内置忽略规则，跳过 >1MB / 二进制 / 非 UTF-8 文件。
2. **diff** 对比本地 `.hce/index.json`：快路径靠 size+mtime 判定未变；慢路径才算 sha256 验证内容。
3. 变更文件按 batch（默认 50 文件 / 5 MiB）并发 `POST /index/upsert`；删除文件走 `/index/delete`。
4. 服务端做 **chunk 级增量**：切分 → 算每个 chunk 的 content sha256 → 查 Milvus 已有 chunk → 命中 hash 的复用（跳过 embedding），新 chunk 批量 embed + insert，消失的旧 chunk delete。
5. 全部 batch 完成后客户端触发**一次** `/index/flush`，避免 Milvus compaction storm。

### search（混合检索）

1. dense 向量召回 `top_k × 3`（多召回给 keyword 命中留空间）。
2. keyword 倒排命中 query 中的字面 token。
3. **merge**：向量 + keyword 双命中的 chunk 加 TF boost（cap 0.20）；仅 keyword 命中的回查元数据后以较低基础分（cap 0.50）作补充召回。
4. 按 final score 排序取 `top_k`。

---

## 🔌 端口拓扑

| 组件 | 端口 | 说明 |
|------|------|------|
| `hce-server` | **9527** | 后端 HTTP API（`config.yaml` 的 `server.port`） |
| `hce-web`（nginx） | **9528** → 80 | 前端 + 反代：`/api/` → `hce-server:9527` |
| Milvus | 19530 | 向量库 |

> CLI 默认 base URL 是 `http://localhost:9528/api/v1`，即默认走 nginx 反代而非直连后端。直连后端调试时用 `--base-url http://localhost:9527/api/v1`。

---

## 🚀 快速开始

### 方式一：docker compose 一键起全栈（推荐）

```bash
# 1. 填入 embedding API key
cp .env.example .env
# 编辑 .env，至少填 HCE_EMBEDDING_API_KEY

# 2. 拉起 etcd + minio + milvus + hce-server + hce-web
docker compose up -d --build

# 3. 浏览器访问前端
open http://localhost:9528
```

### 方式二：本地开发

前置条件：Go 1.25+、已开启 **CGO**（tree-sitter 是 cgo 绑定）、Milvus 已运行在 `localhost:19530`。

```bash
# 启动后端
export HCE_EMBEDDING_API_KEY=<your-key>
go run ./cmd/server -config configs/config.yaml

# 编译 CLI
go build -o bin/hce-cli ./cmd/hce-cli/

# 在任意项目里同步并搜索
cd /path/to/your/project
/path/to/hce/bin/hce-cli search "用户登录鉴权逻辑" --base-url http://localhost:9527/api/v1
```

---

## 🖥️ CLI 用法

```bash
hce-cli sync                       # 扫描并把变更推送到服务端
hce-cli search <query> [-k 10] [-f text|json] [--no-sync]   # 语义搜索（默认先 sync）
hce-cli status                     # 当前 codebase 配置 / 上次 sync
hce-cli list                       # 列出服务端所有已索引 collection
hce-cli clear                      # 清除当前 codebase 的服务端索引 + 本地 state
hce-cli init [--id <name>]         # 显式初始化 .hce/config.json
```

**通用选项**

| 选项 | 默认 | 说明 |
|------|------|------|
| `-p <path>` | 自动查找 | 指定项目根（默认从当前目录向上找 `.hce` 或 `.git`） |
| `--base-url <url>` | `http://localhost:9528/api/v1` | 覆盖服务端地址（亦可用 `HCE_BASE_URL`） |
| `-k <int>` | 5 | search 的 top_k |
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

### 关键环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `HCE_EMBEDDING_API_KEY` | ✅ | 选定供应商的 API Key（`GEMINI_API_KEY` 亦可） |
| `HCE_EMBEDDING_PROVIDER` | | `gemini` / `openai` / `voyageai` / `ollama` |
| `HCE_EMBEDDING_MODEL` | | embedding 模型名 |
| `HCE_EMBEDDING_BASE_URL` | | 自托管 / 代理端点（容器内访问宿主用 `http://host.docker.internal:1234/v1`） |
| `HCE_EMBEDDING_DIM` | | 自托管非 OpenAI 模型需手动指定维度（如 Qwen3-Embedding-8B=4096） |
| `MILVUS_ADDRESS` | | Milvus 地址，默认 `localhost:19530` |

> ⚠️ **切换 provider/model 改了向量维度后，旧 collection 必须 `clear` 重建**，否则 insert/search 维度不匹配报错。

---

## 📡 HTTP API

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

---

## 🧩 支持的语言

tree-sitter AST 切分支持 16 种语言：

`Go` · `Python` · `JavaScript` · `TypeScript` · `Java` · `C` · `C++` · `C#` · `Ruby` · `Rust` · `PHP` · `Kotlin` · `Scala` · `Swift` · `Lua` · `Bash`

---

## 📁 项目结构

```
hce/
├── cmd/
│   ├── server/         # hce-server 入口（embedding/vectordb 工厂、HTTP 服务）
│   └── hce-cli/        # 命令行客户端
├── internal/
│   ├── client/         # 客户端逻辑：scanner / sync / http / codebase / state
│   ├── api/            # HTTP handler + router（push 模式端点）
│   ├── indexer/        # 核心编排：增量索引算法 + hybrid search
│   ├── splitter/       # tree-sitter 多语言 AST 分块
│   ├── embedding/      # 可插拔 embedding 供应商
│   ├── keyword/        # 内存倒排索引（lazy 重建）
│   ├── vectordb/       # VectorDB 接口 + Milvus 实现（IVF_FLAT, COSINE）
│   └── config/         # 服务端 YAML 配置 + 环境变量覆盖
├── pkg/model/          # client 与 server 共享的数据类型
├── web/                # Vite + React + TS + Tailwind 前端
├── configs/            # config.yaml
├── docker-compose.yml  # 一键全栈
└── Dockerfile          # hce-server 镜像
```

---

## 🛠️ 开发

```bash
# 编译服务端（CGO_CFLAGS 抑制上游 lua parser 的 NUL 字面量警告）
CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

# 编译 CLI
go build -o bin/hce-cli ./cmd/hce-cli/

# 静态检查 / 编译验证
go vet ./...
go build ./...
```

### 实现要点（改代码前必读）

- **codebase_id → collection**：`hce_` + sha256(codebase_id)[:24]；chunk 主键 = sha256(relpath + ":" + content_hash)[:32]。content-hash 去重是省 embedding 成本的核心。
- **Milvus VARCHAR 上限 65535 字节**：content 入库前 `truncateOversized` 截到 60000，超长 chunk 必须硬切，否则整批 insert 失败。
- **非 UTF-8 必须 sanitize**：grpc marshal 遇非法 UTF-8 会拒收整批，`sanitizeUTF8` 把坏字节替换为 `�`；客户端侧也直接跳过非 UTF-8 文件。
- **embedding 非对称编码**：索引侧用 `TaskDocument`，查询侧用 query task type。索引与查询的 task type 必须配对，不要混用。
- **CGO 必需**：tree-sitter 是 cgo 绑定，`CGO_ENABLED=0` 无法编译。

---

## 📝 约定

> 代码注释与提交信息均为中文，请保持一致。

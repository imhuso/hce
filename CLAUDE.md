# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> 代码注释与提交信息均为中文，请保持一致。

## 项目概览

HCE 是一个**代码语义检索引擎**，采用 **client-server + push 模式**：
- **客户端（`hce`）** 在本地扫描代码库、做增量 diff，只把变更文件的**内容**推送到服务端。客户端持有源码，服务端从不读文件系统。
- **服务端（`hce-server`）** 接收文件内容，负责切分（tree-sitter）、去重、向量化（embedding）、写入向量库（Milvus）和混合检索。一个 HTTP 服务面向多个 codebase，按 `codebase_id` 隔离到不同 Milvus collection。

## 常用命令

```bash
# 本地起服务端（需要 Milvus 已在 localhost:19530；cgo 必须开启）
go run ./cmd/server -config configs/config.yaml

# 编译服务端（tree-sitter 依赖 cgo；CGO_CFLAGS 抑制上游 lua parser 的 NUL 字面量警告）
CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

# 编译 CLI
go build -o bin/hce ./cmd/hce/

# 一键起全栈（etcd + minio + milvus + hce-server + hce-web）
# 先 cp .env.example .env 填入 HCE_EMBEDDING_API_KEY
docker compose up -d --build

# 静态检查 / 编译验证（仓库目前没有单元测试）
go vet ./...
go build ./...
```

### CLI 用法

```bash
hce sync                       # 扫描并把变更推到服务端
hce search <query> [-k 10] [-f text|json] [--no-sync]   # 语义搜索（默认先 sync）
hce status                     # 当前 codebase 配置 / 上次 sync
hce list                       # 列出服务端所有已索引 collection
hce clear                      # 清除当前 codebase 的服务端索引 + 本地 state
hce init [--id <name>]         # 显式初始化 .hce/config.json
# 通用：-p <path> 指定项目根；--base-url / HCE_BASE_URL 覆盖服务端地址
```

## 端口拓扑（关键，易混）

| 组件 | 端口 | 说明 |
|------|------|------|
| `hce-server` | **9527** | 后端 HTTP API（`config.yaml` 里的 `server.port`） |
| `hce-web`（nginx） | **9528**:80 | 前端 + 反代：`/api/` → `hce-server:9527` |
| Milvus | 19530 | 向量库 |

CLI 默认 base URL 是 **`http://localhost:9528/api/v1`**，即默认走 nginx 反代而非直连后端。直连后端调试时用 `--base-url http://localhost:9527/api/v1`。

## 架构与数据流

### sync（客户端 → 服务端，`internal/client/sync.go` + `internal/indexer/indexer.go`）
1. `Scan` 遍历 root（默认扩展名白名单 + `.gitignore` + `.hceignore` + 内置忽略规则，跳过 >1MB / 二进制 / 非 UTF-8 文件）。`.hceignore` 只读项目根那一个、规则叠加在默认+`.gitignore` 之上、**不支持 `!` 反忽略**（见 `scanner.go` 的 `loadIgnoreFile`/`patternMatch`）。
2. diff 对比 `.hce/index.json`：**快路径**靠 size+mtime 判定未变（大库秒过）；**慢路径**才算 sha256 验证内容。
3. 变更文件按 batch（默认 50 文件 / 5 MiB）并发 `POST /index/upsert`；已删除文件走 `/index/delete`。
4. 服务端 `IndexFiles` 做**chunk 级增量**：切分 → 算每个 chunk 的 content sha256 → 查 Milvus 已有 chunk → 命中 hash 的复用（跳过 embedding），新 chunk 批量 embed + insert，消失的旧 chunk delete。
5. 全部 batch 完成后客户端触发**一次** `/index/flush`（中途不 flush，避免 Milvus compaction storm）。

### search（`Indexer.Search`，hybrid 检索）
1. dense 向量召回 `top_k * 3`（多召回给 keyword 命中留空间）。
2. keyword 倒排（`internal/keyword`）命中 query 中的字面 token。
3. merge：向量+keyword 双命中的 chunk 加 TF boost（cap 0.20）；仅 keyword 命中的回查元数据后以较低基础分（cap 0.50）作为补充召回。
4. 按 final score 排序取 `top_k`。

### 模块职责（`internal/`）
- `client/` — 全部客户端逻辑：`scanner`（扫描+忽略规则）、`sync`（diff+批量推送+限速）、`http`（API client）、`codebase`（`.hce/config.json`、codebase_id 派生、项目根查找）、`state`（`.hce/index.json`）。
- `api/` — HTTP `handler` + `router`（push 模式端点，统一 `apiResponse{code,message,data}`）。
- `indexer/` — 核心编排：增量索引算法 + hybrid search；per-collection 写锁串行化。
- `splitter/` — tree-sitter AST 多语言分块（16 种语言），含小文件整体成块、大节点递归、按行回退等策略。
- `embedding/` — 可插拔供应商（`openai` / `ollama` / `voyageai` / `gemini`），通过 `main.go` 的 `initEmbedding` 工厂选择。
- `keyword/` — 内存倒排索引（不持久化，首次 search 时从 Milvus lazy 重建，写入时增量维护）。
- `vectordb/` — `VectorDB` 接口 + Milvus 实现（IVF_FLAT，COSINE）。
- `config/` — 服务端 YAML 配置 + 环境变量覆盖。
- `pkg/model/` — client 与 server 共享的数据类型。

## 关键约定与坑（改代码前必读）

- **codebase_id → collection**：`hce_` + sha256(codebase_id)[:24]。chunk 主键 = sha256(relpath + ":" + content_hash)[:32]。**content-hash 去重**是省 embedding 成本的核心——只有内容真正变化的 chunk 才会重新 embed。
- **Milvus VARCHAR 上限**：content 字段 schema 上限 65535 字节，入库前 `truncateOversized` 截到 60000。任何超长 chunk（如 minified JS 单行 400KB）必须硬切，否则**整批 insert 失败**。
- **非 UTF-8 必须 sanitize**：grpc marshal 遇到非法 UTF-8 会拒收**整批**，`sanitizeUTF8` 把坏字节替换为 `�`。客户端侧也会直接跳过非 UTF-8 文件。
- **embedding 非对称编码**：索引侧用 `EmbedTyped(..., TaskDocument)`，查询侧用 `Embed()`（对应 query task type）。Gemini/Voyage 借此进入更优向量空间，**索引与查询的 task type 必须配对**，不要混用。
- **embedding 维度**：collection 用 `embedding.Dimension()` 建表。**切换 provider/model 改了维度后，旧 collection 必须 `clear` 重建**，否则 insert/search 维度不匹配报错。自托管模型（LM Studio/vLLM/Ollama）需手动配 `HCE_EMBEDDING_DIM`。
- **keyword 索引不持久化**：`>16384 chunks` 的 collection 目前 lazy 重建只拉一页（见 `ensureKwLoaded` 的 TODO），超大库的 keyword 召回会不全。
- **容错语义**：单 batch 失败只记入 `FailedBatches` 不中断 sync；失败率 >50% 才整体报错。`state` 每批成功后落盘，中断可续传。
- **密钥只走环境变量**：API key 用 `HCE_EMBEDDING_API_KEY` / `GEMINI_API_KEY` 等注入，**不要写进 `configs/config.yaml`**（会进 git）。docker compose 自动读同目录 `.env`。
- **CGO 必需**：tree-sitter 是 cgo 绑定，`CGO_ENABLED=0` 无法编译。

## Web 前端

`web/` 是 Vite + React + TS + Tailwind（shadcn/ui 风格组件在 `web/src/components/ui/`），通过 `web/src/api.ts` 调后端。`web/README.md` 是 Vite 模板自带的样板，非项目文档。

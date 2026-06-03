<div align="right"><b>English</b> | <a href="./README.zh-CN.md">简体中文</a></div>

<h1 align="center">HCE · Code Semantic Search Engine</h1>

<p align="center">Search your codebase in natural language. The client scans and pushes changes locally; the server chunks, embeds, and runs hybrid retrieval.</p>

<p align="center">
  <a href="https://github.com/imhuso/hce/actions/workflows/ci.yml"><img src="https://github.com/imhuso/hce/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/imhuso/hce/releases"><img src="https://img.shields.io/github/v/release/imhuso/hce?include_prereleases&sort=semver&color=blue" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/imhuso/hce" alt="Go version">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License: MIT"></a>
</p>

HCE (Hybrid Code Engine) is a semantic search engine for multiple codebases, using a **client-server + push model**:

- **Client (`hce-cli`)** scans your codebase locally, computes an incremental diff, and pushes only the **content of changed files** to the server. Source code always stays on the client — the server never reads your filesystem.
- **Server (`hce-server`)** receives file content and handles AST chunking (tree-sitter), content dedup, embedding, writing to the vector store (Milvus), and hybrid retrieval. One HTTP service serves many codebases, isolated into separate Milvus collections by `codebase_id`.

A companion web frontend (`hce-web`) lets you search across codebases in the browser.

---

## ✨ Features

- **Hybrid retrieval** — dense vector recall + keyword inverted index. Chunks hit by both get a TF boost; keyword-only hits are added as supplementary recall, balancing semantic relevance with exact matching.
- **Incremental indexing** — chunk-level content-hash dedup: only chunks whose content actually changed get re-embedded, drastically cutting vectorization cost.
- **Fast/slow dual-path diff** — size+mtime quickly rules out unchanged files (large repos pass in seconds); only suspected changes are sha256-verified.
- **Multi-language AST chunking** — tree-sitter based, 16 languages, split along function/class boundaries, with whole-file chunks for small files and line-based fallback.
- **Pluggable embeddings** — built-in OpenAI, Gemini, VoyageAI, and Ollama providers; also compatible with self-hosted OpenAI-protocol models (LM Studio / vLLM).
- **Multi-codebase isolation** — one server serves any number of projects by `codebase_id`, fully isolated.
- **Push architecture** — the server never needs access to your code directory; centralize the index service while source stays on each dev machine.
- **Fault-tolerant resume** — a failed batch doesn't abort the sync; state is flushed per batch, so interrupted syncs resume.

---

## 🚀 Quick Start

### Option 1 — full stack via docker compose (recommended)

```bash
# 1. Set your embedding API key
cp .env.example .env
# Edit .env and set at least HCE_EMBEDDING_API_KEY

# 2. Bring up etcd + minio + milvus + hce-server + hce-web
docker compose up -d --build

# 3. Open the frontend
open http://localhost:9528
```

### Option 2 — run the server from source

Prerequisites: Go 1.25+, **CGO enabled** (tree-sitter is a cgo binding), and Milvus running on `localhost:19530`.

```bash
export HCE_EMBEDDING_API_KEY=<your-key>
go run ./cmd/server -config configs/config.yaml
```

---

## 📦 Install the CLI

`hce-cli` is a pure-Go client (**no CGO required**), a single cross-platform binary. Pick one:

```bash
# A. go install (with a Go toolchain; installs to ~/go/bin)
go install github.com/imhuso/hce/cmd/hce-cli@latest

# B. Prebuilt binary from Releases (no Go needed) — darwin/linux/windows × amd64/arm64
#    https://github.com/imhuso/hce/releases — download, extract, put on your PATH.
gh release download --repo imhuso/hce --pattern '*linux_amd64*'

# C. Build from source
go build -o hce-cli ./cmd/hce-cli && sudo mv hce-cli /usr/local/bin/

hce-cli version    # release builds show the tag version; source builds show "dev"
```

Point the CLI at your server once, machine-wide (shared by all projects):

```bash
hce-cli config --base-url http://<server-ip-or-domain>:9528/api/v1
```

---

## 🖥️ CLI Usage

```bash
hce-cli sync                       # scan and push changes to the server
hce-cli search <query> [-k 10] [-f text|json] [--no-sync]   # semantic search (syncs first by default)
hce-cli status                     # current codebase config / last sync
hce-cli list                       # list all indexed collections on the server
hce-cli clear                      # clear this codebase's server index + local state
hce-cli init [--id <name>]         # explicitly initialize .hce/config.json
hce-cli config [--base-url <url>]  # view / set the global server address (~/.hce/config.json)
hce-cli version                    # print version
```

| Option | Default | Notes |
|--------|---------|-------|
| `-p <path>` | auto-detect | Project root (searches upward from the cwd for `.hce` or `.git`) |
| `--base-url <url>` | `http://localhost:9528/api/v1` | Server address. Priority: flag > `HCE_BASE_URL` > project `.hce/config.json` > global `~/.hce/config.json` > built-in default |
| `-k <int>` | 10 | top_k for search |
| `-f text\|json` | text | search output format |
| `--no-sync` | — | skip sync during search, retrieve only |

---

## ⚙️ Configuration

Server config lives in `configs/config.yaml`. **Always inject secrets via environment variables — never put them in the yaml** (it's committed to git).

### Embedding providers

Selected via `HCE_EMBEDDING_PROVIDER`; the factory is `initEmbedding` in `cmd/server/main.go`:

| Provider | Notes |
|----------|-------|
| `gemini` | Google Gemini (default, `gemini-embedding-001`) |
| `openai` | OpenAI, or any OpenAI-protocol-compatible endpoint (LM Studio / vLLM) |
| `voyageai` | VoyageAI |
| `ollama` | local Ollama |

### Which model to pick & free tiers

| Scenario | Recommended | Why | Free tier |
|----------|-------------|-----|-----------|
| **Code retrieval (top pick)** | VoyageAI `voyage-code-3` | Purpose-built for code; best code-search accuracy; 32k context, 1024-dim | **200M tokens free** per account, then $0.18 / 1M |
| **Zero-cost, no credit card** | Gemini `gemini-embedding-001` | Strong general model, usable via Google AI Studio with no card | ≈ 100 RPM / 1,000 requests-per-day |
| **Fully local / offline** | Ollama or LM Studio (`nomic-embed-text`, `Qwen3-Embedding`) | No API cost, source never leaves your machine | Unlimited (bound by your hardware) |
| **Already on OpenAI** | OpenAI `text-embedding-3-small` / `-large` | Most convenient if you already have a key | No free tier ($0.02 / $0.13 per 1M) |

This repo ships `gemini-embedding-001` as the default for zero-cost onboarding. **For serious code search, switch to `voyage-code-3`** — it's noticeably better, and its 200M free tokens index a large codebase many times over (content-hash dedup makes repeat syncs nearly free).

> Free quotas change often (Gemini cut its quotas in late 2025). Verify current limits at [Voyage pricing](https://docs.voyageai.com/docs/pricing) and [Gemini rate limits](https://ai.google.dev/gemini-api/docs/rate-limits). Switching between models of different dimensions (e.g. voyage `1024` ↔ gemini `3072`) requires a `clear` + rebuild.

### Key environment variables

| Variable | Required | Notes |
|----------|----------|-------|
| `HCE_EMBEDDING_API_KEY` | ✅ | API key for the chosen provider (`GEMINI_API_KEY` also works) |
| `HCE_EMBEDDING_PROVIDER` | | `gemini` / `openai` / `voyageai` / `ollama` |
| `HCE_EMBEDDING_MODEL` | | embedding model name |
| `HCE_EMBEDDING_BASE_URL` | | self-hosted / proxy endpoint (`http://host.docker.internal:1234/v1` to reach the host from a container) |
| `HCE_EMBEDDING_DIM` | | dimension for self-hosted non-OpenAI models (e.g. Qwen3-Embedding-8B=4096) |
| `MILVUS_ADDRESS` | | Milvus address, defaults to `localhost:19530` |

---

## 🏗️ How It Works

```
┌─────────────────────────┐      push (changed file content only)   ┌──────────────────────────────┐
│        hce-cli          │  ───────────────────────────────────▶   │          hce-server          │
│   (local, holds source) │   POST /index/upsert  /delete           │  chunk → dedup → embedding → │
│                         │   POST /index/flush                     │  write Milvus → hybrid search│
│  scan → diff → push     │  ◀───────────────────────────────────   │                              │
└─────────────────────────┘            search results               └───────────────┬──────────────┘
                                                                                     │
                                                                  ┌──────────────────┼──────────────────┐
                                                                  ▼                  ▼                  ▼
                                                            Embedding API       Milvus (vectors)   keyword index
```

**sync (client → server)**

1. **Scan** the project root: extension allowlist + `.gitignore` + built-in ignore rules, skipping files >1MB / binary / non-UTF-8.
2. **diff** against the local `.hce/index.json`: fast path rules out unchanged files via size+mtime; slow path sha256-verifies content.
3. Changed files are pushed concurrently in batches (default 50 files / 5 MiB) via `POST /index/upsert`; deleted files go to `/index/delete`.
4. The server does **chunk-level incremental indexing**: chunk → content sha256 → look up existing chunks in Milvus → reuse hash-matched chunks (skip embedding), batch-embed + insert new chunks, delete chunks that disappeared.
5. After all batches complete, the client triggers `/index/flush` **once**, avoiding a Milvus compaction storm.

**search (hybrid retrieval)**

1. Dense vector recall of `top_k × 3` (over-recall leaves room for keyword hits).
2. Keyword inverted index matches literal tokens in the query.
3. **merge**: vector + keyword double-hits get a TF boost (cap 0.20); keyword-only hits are added as supplementary recall with a lower base score (cap 0.50).
4. Sort by final score and take `top_k`.

### Port topology

| Component | Port | Notes |
|-----------|------|-------|
| `hce-server` | **9527** | Backend HTTP API (`server.port` in `config.yaml`) |
| `hce-web` (nginx) | **9528** → 80 | Frontend + reverse proxy: `/api/` → `hce-server:9527` |
| Milvus | 19530 | Vector store |

> The CLI defaults to `http://localhost:9528/api/v1` (through the nginx proxy). To hit the backend directly, use `--base-url http://localhost:9527/api/v1`.

### HTTP API

All endpoints live under `/api/v1` and return a uniform `{code, message, data}`.

| Method | Path | Notes |
|--------|------|-------|
| POST | `/index/upsert` | push file content and incrementally index |
| POST | `/index/delete` | delete the index for given files |
| POST | `/index/flush` | flush to disk (once after a sync batch) |
| DELETE | `/index` | clear an entire codebase |
| GET | `/indexes` | list all indexed collections |
| POST | `/search` | semantic search |
| GET | `/health` | health check |

### Supported languages

`Go` · `Python` · `JavaScript` · `TypeScript` · `Java` · `C` · `C++` · `C#` · `Ruby` · `Rust` · `PHP` · `Kotlin` · `Scala` · `Swift` · `Lua` · `Bash`

---

## 🛠️ Development

```bash
# Build the server (CGO_CFLAGS suppresses the upstream lua parser's NUL-literal warning)
CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

# Build the CLI
go build -o bin/hce-cli ./cmd/hce-cli/

# Static checks (no unit tests yet)
go vet ./...
go build ./...
```

Deep implementation notes and gotchas (Milvus VARCHAR caps, UTF-8 sanitization, asymmetric embedding task types, the codebase_id → collection scheme) live in [`CLAUDE.md`](./CLAUDE.md).

**Releasing** (maintainers): the version is anchored in the root `package.json`. `npm run release` ([bumpp](https://github.com/antfu-collective/bumpp)) bumps + tags + pushes; pushing a `v*` tag triggers `.github/workflows/release.yml`, which cross-compiles hce-cli for five platforms and publishes a GitHub Release.

---

## 🤝 Contributing

Issues and PRs are welcome. CI runs `go vet` + `go build` on every push/PR, so make sure both pass locally first. The existing codebase is commented in Chinese; contributions in **either Chinese or English** are fine.

## 📄 License

[MIT](./LICENSE) © imhuso

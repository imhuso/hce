<div align="right"><b>English</b> | <a href="./README.zh-CN.md">简体中文</a></div>

<h1 align="center">HCE · Semantic Code Search Engine</h1>

<p align="center">Search your codebase in natural language. The client scans and pushes changes locally; the server chunks, embeds, and runs hybrid retrieval.</p>

<p align="center">
  <a href="https://github.com/imhuso/hce/actions/workflows/ci.yml"><img src="https://github.com/imhuso/hce/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/imhuso/hce/releases"><img src="https://img.shields.io/github/v/release/imhuso/hce?include_prereleases&sort=semver&color=blue" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/imhuso/hce" alt="Go version">
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License: MIT"></a>
</p>

HCE (Hybrid Code Engine) is a semantic search engine for multiple codebases, using a **client-server + push model**:

- **Client (`hce`)** scans your codebase locally, computes an incremental diff, and pushes only the **content of changed files** to the server. Source code always stays on the client — the server never reads your filesystem.
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

One command — creates `.env` if missing, brings the stack up, waits until the backend is healthy, then prints the next steps:

```bash
make up
```

Or do it by hand:

```bash
# 1. Set your embedding API key
cp .env.example .env
# Edit .env and set at least HCE_EMBEDDING_API_KEY

# 2. Bring up etcd + minio + milvus + hce-server + hce-web
docker compose up -d --build

# 3. Open the frontend
open http://localhost:9528
```

> **No API key?** Try it fully local with zero keys: install [Ollama](https://ollama.com), `ollama pull nomic-embed-text`, then in `.env` set `HCE_EMBEDDING_PROVIDER=ollama`, `HCE_EMBEDDING_MODEL=nomic-embed-text`, `HCE_EMBEDDING_BASE_URL=http://host.docker.internal:11434` (see the bottom of `.env.example`). Your source never leaves your machine.

> The stack alone gives you the server + web UI — the UI stays empty until you install the `hce` CLI (below) and run `hce sync` in a project.

### Option 2 — run the server from source

Prerequisites: Go 1.25+, **CGO enabled** (tree-sitter is a cgo binding), and Milvus running on `localhost:19530`.

```bash
export HCE_EMBEDDING_API_KEY=<your-key>
go run ./cmd/server -config configs/config.yaml
```

---

## 📦 Install the CLI

`hce` is a pure-Go client (**no CGO required**), a single cross-platform binary. Pick one:

```bash
# A. go install (with a Go toolchain; installs to ~/go/bin)
go install github.com/imhuso/hce/cmd/hce@latest

# B. Prebuilt binary from Releases (no Go needed) — darwin/linux/windows × amd64/arm64
#    https://github.com/imhuso/hce/releases — download, extract, put on your PATH.
gh release download --repo imhuso/hce --pattern '*linux_amd64*'

# C. Build from source
go build -o hce ./cmd/hce && sudo mv hce /usr/local/bin/

hce version    # release builds show the tag version; source builds show "dev"
```

Point the CLI at your server once, machine-wide (shared by all projects):

```bash
hce config --base-url http://<server-ip-or-domain>:9528/api/v1
```

---

## 🖥️ CLI Usage

```bash
hce sync                       # scan and push changes to the server
hce search <query> [-k 10] [-f text|json] [--no-sync]   # semantic search (syncs first by default)
hce status                     # current codebase config / last sync
hce list                       # list all indexed collections on the server
hce clear                      # clear this codebase's server index + local state
hce init [--id <name>]         # explicitly initialize .hce/config.json
hce config [--base-url <url>]  # view / set the global server address (~/.hce/config.json)
hce version                    # print version
```

| Option | Default | Notes |
|--------|---------|-------|
| `-p <path>` | auto-detect | Project root (searches upward from the cwd for `.hce` or `.git`) |
| `--base-url <url>` | `http://localhost:9528/api/v1` | Server address. Priority: flag > `HCE_BASE_URL` > project `.hce/config.json` > global `~/.hce/config.json` > built-in default |
| `-k <int>` | 10 | top_k for search |
| `-f text\|json` | text | search output format |
| `--no-sync` | — | skip sync during search, retrieve only |

---

## 🤖 Use with Claude Code

HCE ships a [Claude Code](https://www.anthropic.com/claude-code) **skill** (`.claude/skills/hce`): ask about your code in plain language and the agent runs `hce search` for you — no commands to memorize. The first time around, it also walks you through setup.

### Quick install — let the AI do it

Paste this into Claude Code; it will fetch the skill, install the CLI, and wire everything up:

```text
Set up HCE semantic code search for me:
1. Install the CLI: run `go install github.com/imhuso/hce/cmd/hce@latest` (or download
   a binary from https://github.com/imhuso/hce/releases and put it on PATH); verify with
   `hce version`.
2. Install the skill: copy `.claude/skills/hce` from https://github.com/imhuso/hce into
   `~/.claude/skills/hce`.
3. Point me at a server: ask where my HCE backend is (local docker / LAN IP / public
   domain) and run `hce config --base-url <url>`. If I don't have one, tell me to run
   `make up` in the hce repo.
4. Index this project: run `hce sync`, then confirm with a sample `hce search`.
Report the codebase_id, the effective server URL, and the indexed file count when done.
```

### Manual install

```bash
go install github.com/imhuso/hce/cmd/hce@latest      # CLI (see "Install the CLI" above)
cp -r .claude/skills/hce ~/.claude/skills/hce        # skill — global, usable in any project
# or drop it in <your-project>/.claude/skills/hce for that project only
```

Then just ask Claude Code in natural language:

> where is the order recipient's phone number masked?
> how is the JWT validation interceptor implemented?

The skill triggers automatically on "where is X / how is X implemented" questions (or call it explicitly with `/hce`), runs an incremental sync, and returns the relevant snippets. Destructive ops (`hce clear` / full rebuild) always confirm with you first.

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

### Excluding files from indexing (`.hceignore`)

The scanner already skips your `.gitignore` entries plus built-in noise (`node_modules`, `dist`, lockfiles, minified assets, dot-directories, …). To exclude things that *are* tracked in git but you don't want indexed — test fixtures, generated code, vendored snapshots, large docs — add a `.hceignore` at the project root (next to `.hce/`):

```gitignore
# .hceignore — lives at the project root
testdata/
**/__generated__/
*.snapshot
docs/legacy/
```

Syntax is a practical subset of `.gitignore`: `#` comments and blank lines are skipped; a single segment (`*.snapshot`) matches at any depth; multi-segment patterns (`docs/legacy`) match by path prefix; a leading `/` anchors to the root; `**` spans directories; trailing `/` and `/**` are equivalent. Two differences from git to keep in mind:

- **Root-level only** — only the single `.hceignore` at the project root is read (no nested per-directory files).
- **Additive only** — it can only *add* exclusions; `!pattern` re-include is **not** supported, so it cannot bring back a file already excluded by `.gitignore` or the built-in rules.

Commit `.hceignore` to share the same indexing scope across your team.

---

## 🏗️ How It Works

```
┌─────────────────────────┐      push (changed file content only)   ┌──────────────────────────────┐
│        hce              │  ───────────────────────────────────▶   │          hce-server          │
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

1. **Scan** the project root: extension allowlist + `.gitignore` + `.hceignore` + built-in ignore rules, skipping files >1MB / binary / non-UTF-8.
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
go build -o bin/hce ./cmd/hce/

# Static checks (no unit tests yet)
go vet ./...
go build ./...
```

Deep implementation notes and gotchas (Milvus VARCHAR caps, UTF-8 sanitization, asymmetric embedding task types, the codebase_id → collection scheme) live in [`CLAUDE.md`](./CLAUDE.md).

**Releasing** (maintainers): the version is anchored in the root `package.json`. `npm run release` ([bumpp](https://github.com/antfu-collective/bumpp)) bumps + tags + pushes; pushing a `v*` tag triggers `.github/workflows/release.yml`, which cross-compiles hce for five platforms and publishes a GitHub Release.

---

## 🤝 Contributing

Issues and PRs are welcome. CI runs `go vet` + `go build` on every push/PR, so make sure both pass locally first. The existing codebase is commented in Chinese; contributions in **either Chinese or English** are fine.

## 📄 License

[MIT](./LICENSE) © imhuso

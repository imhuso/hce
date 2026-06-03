# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> Write code comments and commit messages in English; keep it consistent.

## Project overview

HCE is a **code semantic search engine** built on a **client-server + push model**:
- **Client (`hce`)** scans the codebase locally, computes an incremental diff, and pushes only the **content** of changed files to the server. The client owns the source; the server never reads the filesystem.
- **Server (`hce-server`)** receives file content and handles chunking (tree-sitter), dedup, embedding, writing to the vector store (Milvus), and hybrid retrieval. A single HTTP service serves many codebases, isolating each into a separate Milvus collection by `codebase_id`.

## Common commands

```bash
# Run the server locally (requires Milvus at localhost:19530; cgo must be enabled)
go run ./cmd/server -config configs/config.yaml

# Build the server (tree-sitter needs cgo; CGO_CFLAGS suppresses the upstream lua parser's NUL-literal warning)
CGO_ENABLED=1 CGO_CFLAGS="-Wno-null-character" go build -ldflags="-s -w" -o hce-server ./cmd/server/

# Build the CLI
go build -o bin/hce ./cmd/hce/

# Bring up the full stack (etcd + minio + milvus + hce-server + hce-web)
# First: cp .env.example .env and fill in HCE_EMBEDDING_API_KEY
docker compose up -d --build

# Static checks / build verification (the repo currently has no unit tests)
go vet ./...
go build ./...
```

### CLI usage

```bash
hce sync                       # Scan and push changes to the server
hce search <query> [-k 10] [-f text|json] [--no-sync]   # Semantic search (syncs first by default)
hce status                     # Current codebase config / last sync
hce list                       # List all indexed collections on the server
hce clear                      # Clear the current codebase's server index + local state
hce init [--id <name>]         # Explicitly initialize .hce/config.json
# Common flags: -p <path> sets the project root; --base-url / HCE_BASE_URL overrides the server address
```

## Port topology (important, easy to confuse)

| Component | Port | Notes |
|------|------|------|
| `hce-server` | **9527** | Backend HTTP API (`server.port` in `config.yaml`) |
| `hce-web` (nginx) | **9528**:80 | Frontend + reverse proxy: `/api/` → `hce-server:9527` |
| Milvus | 19530 | Vector store |

The CLI's default base URL is **`http://localhost:9528/api/v1`**, i.e. it goes through the nginx reverse proxy by default rather than hitting the backend directly. To debug against the backend directly, use `--base-url http://localhost:9527/api/v1`.

## Architecture and data flow

### sync (client → server, `internal/client/sync.go` + `internal/indexer/indexer.go`)
1. `Scan` walks the root (default extension allowlist + `.gitignore` + `.hceignore` + built-in ignore rules; skips files >1MB / binary / non-UTF-8). Only the single `.hceignore` at the project root is read; its rules stack on top of the defaults + `.gitignore`, and **`!` negation is not supported** (see `loadIgnoreFile`/`patternMatch` in `scanner.go`).
2. diff against `.hce/index.json`: the **fast path** treats a file as unchanged by size+mtime (large repos pass in seconds); the **slow path** computes a sha256 to verify content.
3. Changed files are pushed concurrently in batches (default 50 files / 5 MiB) via `POST /index/upsert`; deleted files go to `/index/delete`.
4. The server's `IndexFiles` does **chunk-level incremental** indexing: chunk → compute each chunk's content sha256 → look up existing chunks in Milvus → reuse chunks whose hash matches (skip embedding), batch-embed + insert new chunks, delete chunks that disappeared.
5. After all batches complete, the client triggers `/index/flush` **once** (it does not flush mid-run, to avoid a Milvus compaction storm).

### search (`Indexer.Search`, hybrid retrieval)
1. Dense vector recall of `top_k * 3` (over-recall to leave room for keyword hits).
2. Keyword inverted index (`internal/keyword`) matches literal tokens in the query.
3. merge: chunks hit by both vector + keyword get a TF boost (cap 0.20); chunks hit by keyword only are looked up for metadata and added as supplementary recall with a lower base score (cap 0.50).
4. Sort by final score and take `top_k`.

### Module responsibilities (`internal/`)
- `client/` — all client logic: `scanner` (scanning + ignore rules), `sync` (diff + batched push + rate limiting), `http` (API client), `codebase` (`.hce/config.json`, codebase_id derivation, project-root lookup), `state` (`.hce/index.json`).
- `api/` — HTTP `handler` + `router` (push-model endpoints, unified `apiResponse{code,message,data}`).
- `indexer/` — core orchestration: the incremental indexing algorithm + hybrid search; serialized via a per-collection write lock.
- `splitter/` — tree-sitter AST chunking across multiple languages (16), with strategies like whole-file chunks for small files, recursion for large nodes, and line-based fallback.
- `embedding/` — pluggable providers (`openai` / `ollama` / `voyageai` / `gemini`), selected by the `initEmbedding` factory in `main.go`.
- `keyword/` — in-memory inverted index (not persisted; lazily rebuilt from Milvus on first search, maintained incrementally on writes).
- `vectordb/` — the `VectorDB` interface + Milvus implementation (IVF_FLAT, COSINE).
- `config/` — server YAML config + environment-variable overrides.
- `pkg/model/` — data types shared between client and server.

## Key conventions and gotchas (read before changing code)

- **codebase_id → collection**: `hce_` + sha256(codebase_id)[:24]. chunk primary key = sha256(relpath + ":" + content_hash)[:32]. **content-hash dedup** is the core of saving embedding cost — only chunks whose content actually changed get re-embedded.
- **Milvus VARCHAR limit**: the content field's schema caps at 65535 bytes; `truncateOversized` cuts to 60000 before insert. Any oversized chunk (e.g. a single 400KB minified-JS line) must be hard-cut, otherwise the **entire batch insert fails**.
- **Non-UTF-8 must be sanitized**: grpc marshal rejects the **whole batch** on invalid UTF-8; `sanitizeUTF8` replaces bad bytes with `�`. The client side also skips non-UTF-8 files outright.
- **Asymmetric embedding encoding**: the index side uses `EmbedTyped(..., TaskDocument)`, the query side uses `Embed()` (the query task type). Gemini/Voyage use this to reach a better vector space, so **the index and query task types must be paired** — do not mix them.
- **Embedding dimension**: collections are created with `embedding.Dimension()`. **After switching provider/model changes the dimension, the old collection must be `clear`ed and rebuilt**, otherwise insert/search fail on a dimension mismatch. Self-hosted models (LM Studio/vLLM/Ollama) require setting `HCE_EMBEDDING_DIM` manually.
- **Keyword index is not persisted**: for collections with `>16384 chunks`, the lazy rebuild currently fetches only one page (see the TODO in `ensureKwLoaded`), so keyword recall is incomplete for very large repos.
- **Fault-tolerance semantics**: a single failed batch is only recorded in `FailedBatches` and does not abort the sync; only a failure rate >50% errors out overall. `state` is persisted after each successful batch, so an interrupted sync can resume.
- **Secrets go through environment variables only**: inject API keys via `HCE_EMBEDDING_API_KEY` / `GEMINI_API_KEY` etc.; **do not put them in `configs/config.yaml`** (it's committed to git). docker compose reads the `.env` in the same directory automatically.
- **CGO required**: tree-sitter is a cgo binding; `CGO_ENABLED=0` cannot build.

## Web frontend

`web/` is Vite + React + TS + Tailwind (shadcn/ui-style components in `web/src/components/ui/`), calling the backend via `web/src/api.ts`. `web/README.md` is the stock Vite template boilerplate, not project documentation.

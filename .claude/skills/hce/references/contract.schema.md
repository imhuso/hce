# Contract: command cheatsheet / exit codes

The server address is resolved by layered config (see setup.md); query tips and the mandatory `clear` constraint are in SKILL.md.

## Commands

```bash
hce search "<query>" [-k 5] [-f text|json] [--no-sync]  # semantic search (incremental sync first by default)
hce sync          # build / incrementally update the index (full on first run, then size+mtime fast path + sha256 slow path)
hce status        # codebase config / effective base_url / online·offline / last sync
hce config [--base-url <url>]    # view / write the global address (~/.hce/config.json)
hce init [--id <name>]           # explicitly initialize .hce/config.json (ID derived from the project path by default)
hce list          # list all indexed collections on the server
hce clear         # clear the current codebase's index (⚠ requires user consent)
```

## search flags
- `-k`: number of results, default 5
- `-f`: `text` (default) / `json`
- `--no-sync`: skip incremental sync, search only
- `--base-url` / `HCE_BASE_URL`: one-off override of the server address

## output / exit codes
- **stdout**: non-empty when there are results — `Path: <relative path>` + a code snippet with line numbers (text format)
- **non-zero exit**: usage error / `hce` not on PATH / backend unreachable / search failed

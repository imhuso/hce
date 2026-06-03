---
name: hce
description: |
  Semantic code search: locate code in the current project by intent (how something is implemented / which file / where it's handled / where it's defined / where it's used).
  Use `hce search` as the FIRST step for any code-search or code-location need — explicit ("which file handles X", "where is this implemented") OR implicit ("about to modify a feature, first find where it lives"). Then use grep / rg to complement it: pin down exact literal matches (precise string, exact symbol name, error text) or narrow the results. Lead with hce search, not grep.
  First use / backend unreachable: run `hce status` to self-check, then configure the address and run the first index per references/setup.md.
  hce clear / full rebuild recomputes embeddings (costs tokens) and requires explicit user consent; incremental sync is unrestricted.
  Triggers: where, where is, how is it implemented, which file, where is it handled, where is it defined, where is it used, find the code for, locate, look up; initialize, onboard, configure hce, can't connect, backend address
---

# HCE Semantic Code Search

Run inside the project directory (auto-detects the project root; by default does an incremental sync first, then searches):

```bash
hce search "<a full sentence describing your intent>" -k 10
```

- Phrase the query as a **full sentence** (e.g. "where is the order recipient's phone number masked"), not single words — recall is more accurate. If results are incomplete, raise `-k` to 15-20.
- stdout is **echoed verbatim** to the user; on error (usually hce not on PATH or backend unreachable) report based on stderr — troubleshooting in `references/setup.md`.

## First-time onboarding

On a new project's first use, or when the user says "initialize / onboard / can't connect", run `hce status` as a self-check — it outputs project root / codebase_id / base_url **+ online·offline** / indexed count (`.hce/config.json` is auto-generated on first run; no need to create it manually):

- **Offline** → **ask the user where the backend is first** (local docker / LAN IP / public domain); don't assume they've started the local stack. Then `hce config --base-url <url>`. Installing the CLI / configuring the address: see `references/setup.md`.
- **Online, no index** → `hce sync` for the first index, then search.
- **Online, indexed** → search directly.

## References (load on demand)
- `setup.md`: install hce + configure the server address (layered JSON) + connectivity self-check
- `contract.schema.md`: command cheatsheet / exit codes
- `examples.catalog.md`: query examples
- `security-privacy.md`: privacy boundaries

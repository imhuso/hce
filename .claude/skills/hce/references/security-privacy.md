# Security & Privacy

- **Source never leaves your machine**: push model — `hce` scans locally and only sends changed-file content to hce-server; the server never reads your disk.
- **Embedding sends code chunks to the chosen provider** to compute vectors (using self-hosted models like ollama / LM Studio keeps everything inside your network). Provider credentials live in the **server's** `.env` (gitignored) — don't commit them.
- **Connecting to a public backend** means code content travels over the public internet — make sure the backend is trusted, use HTTPS, and add auth on the endpoints.
- **Client-side footprint**: `hce` writes `.hce/` at the project root (codebase config + local state); ignore `.hce/` in the project `.gitignore`. The global address is stored in `~/.hce/config.json`, which contains only the URL.
- **Restrained output**: keep only relevant, concise snippets from search results; don't echo sensitive information into user-visible output.

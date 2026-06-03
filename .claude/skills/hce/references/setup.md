# Setup: install hce + configure the server address

This skill drives `hce` directly, with no scripts. It needs: a reachable **hce-server backend** + the **hce** binary on PATH.

## 1. Install hce (single cross-platform binary, no CGO needed)

```bash
go install github.com/imhuso/hce/cmd/hce@latest   # with Go: installs to ~/go/bin
# or download a prebuilt binary: gh release download --repo imhuso/hce --pattern '*linux_amd64*', extract, and put it on PATH
# or build from source: go build -o hce ./cmd/hce && sudo mv hce /usr/local/bin/
hce version   # verify
```

## 2. Where the backend comes from (determines the server address)

- **Self-hosted locally (default)**: in the hce repo, `cp .env.example .env` and fill in `HCE_EMBEDDING_API_KEY`, then `docker compose up -d` (etcd+minio+milvus+server+web). Backend on 9527, frontend/reverse-proxy on 9528; the default goes through 9528, so no address config is needed.
- **LAN-shared**: one machine runs the stack, others connect via its LAN IP: `hce config --base-url http://192.168.1.50:9528/api/v1`.
- **Public domain**: reverse-proxy 9528 to a domain (HTTPS + auth recommended): `hce config --base-url https://hce.example.com/api/v1`.

## 3. Address resolution layers (high → low priority)

1. `--base-url` flag / `HCE_BASE_URL` env var (one-off / CI)
2. project `<project>/.hce/config.json`'s `base_url` (a project that needs a different backend)
3. global `~/.hce/config.json` (written by `hce config --base-url`, machine-level default)
4. built-in default `http://localhost:9528/api/v1`

Each user's `~/.hce/` is naturally isolated; project-level overrides global. View the currently effective config: `hce config` (no args).

#!/usr/bin/env bash
# 公共逻辑：加载用户配置、定位 hce-cli、求项目根、组装服务端地址参数。
# 被同目录 search.sh / index.sh 用 source 引入，不单独执行。
set -euo pipefail

SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 1) 加载用户配置：复制 config.example.sh 为同目录 config.sh 后生效。
#    可在其中 export HCE_BASE_URL / HCE_CLI / HCE_TOPK。
if [ -f "$SKILL_DIR/config.sh" ]; then
  # shellcheck disable=SC1091
  . "$SKILL_DIR/config.sh"
fi

# 2) 定位 hce-cli：HCE_CLI 环境变量 > PATH > 常见安装位置。
#    hce-cli 是纯客户端，无需 CGO：go build -o bin/hce-cli ./cmd/hce-cli 即可。
HCE_CLI="${HCE_CLI:-}"
[ -z "$HCE_CLI" ] && HCE_CLI="$(command -v hce-cli 2>/dev/null || true)"
[ -z "$HCE_CLI" ] && [ -x "$HOME/Workspace/code/hce/bin/hce-cli" ] && HCE_CLI="$HOME/Workspace/code/hce/bin/hce-cli"
[ -z "$HCE_CLI" ] && {
  echo "Error: 找不到 hce-cli。请编译（go build -o bin/hce-cli ./cmd/hce-cli，无需 CGO）后放入 PATH，或在 config.sh 设置 HCE_CLI。" >&2
  exit 4
}

# 3) 项目根（兼容 git worktree）：向上找 .git；找不到用当前目录。
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  ROOT="$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")"
else
  ROOT="$PWD"
fi

# 4) 服务端地址：HCE_BASE_URL 非空时透传给 hce-cli（决定连 localhost / 局域网 IP / 公网域名）。
#    不设置则用 hce-cli 默认 http://localhost:9528/api/v1。
BASE_URL_ARGS=()
if [ -n "${HCE_BASE_URL:-}" ]; then
  BASE_URL_ARGS=(--base-url "$HCE_BASE_URL")
fi

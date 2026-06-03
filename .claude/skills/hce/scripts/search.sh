#!/usr/bin/env bash
# 语义检索：调用 hce-cli 对当前项目做自然语言检索（默认先增量 sync 再检索）。
# stdout 即检索结果。服务端地址由 config.sh / HCE_BASE_URL 决定（本机 / 局域网 / 公网）。
# 用法：bash search.sh "你的自然语言查询"
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_common.sh"

if [ "$#" -lt 1 ]; then
  echo "Usage: bash $(dirname "${BASH_SOURCE[0]}")/search.sh \"你的自然语言查询\"" >&2
  exit 2
fi

# flag 必须在 query 之前；top_k 默认 10，可用 HCE_TOPK 覆盖。
exec "$HCE_CLI" search -p "$ROOT" "${BASE_URL_ARGS[@]}" -k "${HCE_TOPK:-10}" "$*"

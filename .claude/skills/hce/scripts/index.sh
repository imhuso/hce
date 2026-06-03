#!/usr/bin/env bash
# 建立/增量更新索引：bash index.sh
# 等价于 hce-cli sync（扫描当前项目变更并推送到 hce-server）。
# 首次全量、之后增量；search 默认也会先 sync，通常无需手动跑。
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/_common.sh"

exec "$HCE_CLI" sync -p "$ROOT" "${BASE_URL_ARGS[@]}"

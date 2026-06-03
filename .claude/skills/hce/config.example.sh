#!/usr/bin/env bash
# HCE skill 配置模板 —— 复制本文件为同目录 config.sh 后按需修改。
# config.sh 会被 scripts 自动加载（已 gitignore，不会进仓库）。仅含 URL/路径，无密钥。

# ── 服务端地址（决定 search/sync 连到哪个 hce 后端）──────────────────────────
# 不设置则用 hce-cli 默认 http://localhost:9528/api/v1（本机 docker compose 起的栈）。
# 端口约定：9528 = nginx 前端反代（推荐）；9527 = 后端直连（调试）。
#
# 【局域网】填部署了 hce 栈那台机器的内网 IP：
# export HCE_BASE_URL="http://192.168.1.50:9528/api/v1"
#
# 【公网】填部署后的域名（建议 https）：
# export HCE_BASE_URL="https://hce.example.com/api/v1"
#
# 【本机】通常无需设置；如需显式指定：
# export HCE_BASE_URL="http://localhost:9528/api/v1"

# ── hce-cli 二进制位置（可选）────────────────────────────────────────────────
# 默认按 PATH > ~/Workspace/code/hce/bin/hce-cli 查找。需要时显式指定：
# export HCE_CLI="$HOME/bin/hce-cli"

# ── 检索返回条数（可选，默认 10；召回不全时调大到 15-20）──────────────────────
# export HCE_TOPK=15

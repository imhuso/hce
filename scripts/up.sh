#!/usr/bin/env bash
# 一键起全栈：确保 .env 存在 → 预检 embedding key → docker compose up → 等后端就绪 → 打印下一步。
# 目的：把「克隆后到底怎么跑起来」从 README 里的 5 步手动操作，收成 `make up` 一条命令。
set -euo pipefail
cd "$(dirname "$0")/.."

HEALTH_URL="http://localhost:9528/api/v1/health"

# 1. 确保 .env 存在。compose 用它注入 embedding 配置；缺了会以空 key 静默起来。
if [ ! -f .env ]; then
  cp .env.example .env
  echo "✓ 已从 .env.example 生成 .env"
  echo "  ⚠ 请编辑 .env 填入 HCE_EMBEDDING_API_KEY（或改用 Ollama 零密钥，见 .env.example 底部），然后重新运行 make up"
  echo "    ${EDITOR:-vi} .env"
  exit 1
fi

# 2. 预检：非 ollama 但 key 为空 → 当场拦下，省得 compose 起来又 fail-fast 崩、再去翻日志。
provider="$(grep -E '^HCE_EMBEDDING_PROVIDER=' .env | tail -1 | cut -d= -f2- | tr -d '[:space:]')"
provider="${provider:-gemini}"
key="$(grep -E '^HCE_EMBEDDING_API_KEY=' .env | tail -1 | cut -d= -f2- | tr -d '[:space:]')"
if [ "$provider" != "ollama" ] && [ -z "$key" ]; then
  echo "✘ .env 里 HCE_EMBEDDING_API_KEY 为空，而 provider=$provider 需要它。"
  echo "  填入 key 后重试，或设 HCE_EMBEDDING_PROVIDER=ollama 走本地零密钥（需本地装 Ollama）。"
  exit 1
fi

# 3. 起栈
echo "▶ docker compose up -d --build（首次会现编 web+server 并拉 milvus，耐心等几分钟）..."
docker compose up -d --build

# 4. 等后端就绪（经 nginx 反代的 9528）。milvus standalone 通常要 30~60s 才健康。
printf "▶ 等待后端就绪 "
for i in $(seq 1 90); do
  if curl -fsS "$HEALTH_URL" >/dev/null 2>&1; then
    echo " ✓ 已就绪"
    break
  fi
  printf "."
  sleep 2
  if [ "$i" -eq 90 ]; then
    echo
    echo "✘ 等待超时（3 分钟）。排查：docker compose logs -f hce-server"
    exit 1
  fi
done

cat <<'EOF'

✅ 全栈已启动
   Web UI : http://localhost:9528

下一步 —— 装 CLI 并索引你的项目：
   go install github.com/imhuso/hce/cmd/hce@latest    # 或从 Releases 下预编译二进制
   hce config --base-url http://localhost:9528/api/v1 # 本机默认即此地址，可省
   cd /your/project
   hce sync                                           # 首次全量索引
   hce search "用一句话描述你要找的代码"
EOF

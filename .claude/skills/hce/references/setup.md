# Setup：装 hce + 配服务端地址

本 skill 直接驱动 `hce`，无脚本。需要：可达的 **hce-server 后端** + PATH 里的 **hce** 二进制。

## 1. 装 hce（单一跨平台二进制，无需 CGO）

```bash
go install github.com/imhuso/hce/cmd/hce@latest   # 有 Go：装到 ~/go/bin
# 或下载预编译：gh release download --repo imhuso/hce --pattern '*linux_amd64*'，解压放进 PATH
# 或源码编译：go build -o hce ./cmd/hce && sudo mv hce /usr/local/bin/
hce version   # 验证
```

## 2. 后端从哪来（决定服务端地址）

- **本机自起（默认）**：在 hce 仓库 `cp .env.example .env` 填 `HCE_EMBEDDING_API_KEY`，`docker compose up -d`（etcd+minio+milvus+server+web）。后端 9527、前端/反代 9528，默认走 9528 无需配地址。
- **局域网共享**：某机起栈，其他人连其内网 IP：`hce config --base-url http://192.168.1.50:9528/api/v1`。
- **公网域名**：把 9528 反代到域名（建议 HTTPS + 鉴权）：`hce config --base-url https://hce.example.com/api/v1`。

## 3. 地址分层解析（高 → 低优先级）

1. `--base-url` 旗标 / `HCE_BASE_URL` 环境变量（一次性 / CI）
2. 项目 `<项目>/.hce/config.json` 的 `base_url`（某项目要连别的后端）
3. 全局 `~/.hce/config.json`（`hce config --base-url` 写入，机器级默认）
4. 内置默认 `http://localhost:9528/api/v1`

各用户 `~/.hce/` 天然隔离；项目级覆盖全局。查看当前生效配置：`hce config`（无参）。

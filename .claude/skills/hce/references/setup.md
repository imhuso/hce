# Setup：装 hce-cli + 配置服务端地址

本 skill 直接驱动 `hce-cli`，不含脚本。需要两样东西：一个可达的 **hce-server 后端**，
和本机 PATH 里的 **hce-cli** 二进制。

## 1. 装 hce-cli（单一跨平台二进制，无需 CGO）

```bash
# A. go install（有 Go，一行，装到 ~/go/bin）
go install github.com/imhuso/hce/cmd/hce-cli@latest

# B. 从 Releases 下载预编译二进制（无需 Go）：选对应平台 .tar.gz/.zip 解压放进 PATH
gh release download --repo imhuso/hce --pattern '*linux_amd64*'

# C. 从源码编译
go build -o hce-cli ./cmd/hce-cli && sudo mv hce-cli /usr/local/bin/

hce-cli version   # 验证
```

## 2. 后端从哪来（决定服务端地址）

三选一：

### A. 本机自起（默认）
```bash
cd /path/to/hce
cp .env.example .env     # 填入 HCE_EMBEDDING_API_KEY
docker compose up -d     # etcd + minio + milvus + hce-server + hce-web
```
后端在 `localhost:9527`，前端/反代在 `localhost:9528`。无需配地址，默认走 9528。

### B. 局域网共享
某台机器按 A 起栈，其他人连它的内网 IP（compose 默认只映射 9528 到宿主，统一走它）：
```bash
hce-cli config --base-url http://192.168.1.50:9528/api/v1
```

### C. 公网域名（已部署）
把 9528 反代到域名（建议 HTTPS + 鉴权）：
```bash
hce-cli config --base-url https://hce.example.com/api/v1
```

## 3. 分层 JSON 配置（按用户、按项目）

地址按优先级解析，高 → 低：

1. `--base-url <url>` 旗标（一次性）
2. `HCE_BASE_URL` 环境变量（一次性 / CI）
3. **项目** `<项目>/.hce/config.json` 的 `base_url`（某项目要连别的后端时填这里）
4. **全局** `~/.hce/config.json` 的 `base_url`（每用户机器级默认，`hce-cli config --base-url` 写入）
5. 内置默认 `http://localhost:9528/api/v1`

- "不同用户"：`~/.hce/` 在各自 home，天然隔离。
- "不同项目"：项目级 `.hce/config.json` 覆盖全局默认。
- 查看当前生效配置与优先级：`hce-cli config`（无参）。

## 4. 首次索引

```bash
cd /your/project
hce-cli sync     # 首次全量、之后增量
```
之后 `hce-cli search` 会自动先增量 sync 再检索，通常无需手动 sync。

# Setup：部署后端 + 配置服务端地址

本 skill 是 hce 的**客户端使用层**。它依赖两样东西：一个可达的 **hce-server 后端**，和一个本机的 **hce-cli** 二进制。

## 1. 后端从哪来

三选一，对应三种 `HCE_BASE_URL`：

### A. 本机自起（默认）
```bash
cd /path/to/hce          # hce 仓库根
cp .env.example .env     # 填入 HCE_EMBEDDING_API_KEY
docker compose up -d     # etcd + minio + milvus + hce-server + hce-web
```
后端在 `localhost:9527`，前端/反代在 `localhost:9528`。`HCE_BASE_URL` 留空即可（默认走 9528）。

### B. 局域网共享
某台机器（开发服务器 / NAS）按 A 起栈，其他人连它的内网 IP：
```bash
# config.sh
export HCE_BASE_URL="http://192.168.1.50:9528/api/v1"
```
> 注意：`docker-compose.yml` 默认只把 **9528** 端口映射到宿主机（`ports: 9528:80`），
> 后端 9527 仅在 compose 内网。局域网访问统一走 9528（nginx 反代到后端），无需额外开端口。

### C. 公网域名（已部署）
把 9528 反代到一个域名（建议加 HTTPS / 鉴权），客户端：
```bash
# config.sh
export HCE_BASE_URL="https://hce.example.com/api/v1"
```

## 2. hce-cli 二进制

```bash
cd /path/to/hce
go build -o bin/hce-cli ./cmd/hce-cli   # 纯客户端，无需 CGO；跨平台可直接交叉编译分发
```
让脚本能找到它（任选其一）：
- 放进 PATH（如 `cp bin/hce-cli /usr/local/bin/`）
- 在 `config.sh` 设 `export HCE_CLI=/abs/path/to/hce-cli`
- 放在默认位置 `~/Workspace/code/hce/bin/hce-cli`

## 3. 首次索引

```bash
cd /your/project
bash /path/to/skill/scripts/index.sh   # = hce-cli sync，首次全量、之后增量
```
之后 `search.sh` 会自动先增量 sync 再检索，通常无需手动 index。

## 配置加载顺序

`scripts/_common.sh` 先 source 同目录 `config.sh`（若存在），再读环境变量。
因此命令行临时 `export HCE_BASE_URL=...` 与 `config.sh` 等效，命令行当次生效。

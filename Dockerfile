# 构建阶段
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
# 抑制上游 tree-sitter 生成的 lua parser.c 中 NUL 字节字面量产生的 cgo 警告
ENV CGO_CFLAGS="-Wno-null-character"
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /hce-server ./cmd/server/

# 运行阶段
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /hce-server /app/hce-server

# 注意：不再把 configs/config.yaml 烤进镜像。
# 通过 volume 挂载 configs/ 或环境变量 (HCE_EMBEDDING_API_KEY 等) 注入。
EXPOSE 9527

ENTRYPOINT ["/app/hce-server"]
CMD ["-config", "/app/configs/config.yaml"]

---
name: hce
description: |
  代码语义检索工具，用于在当前项目下做自然语言检索（search）。
  适用于：需要快速定位相关代码位置、查找实现逻辑。
  输出要求：脚本 stdout 必须原样回显给用户；search 若有结果必须明确提示"已检索到结果"。
  【推荐】涉及代码搜索/定位时，优先使用本 skill 的 search.sh 做语义检索；grep/rg/find 在需要精确字面匹配时仍可使用。
  【强制】清理/全量重建索引（hce-cli clear / 重建）须经用户明确同意，禁止擅自重建以免浪费 token；增量 sync 不受限。
  触发词：哪里、在哪、代码在哪、怎么实现的、哪个文件、什么地方、哪里处理、哪里定义、怎么写的、哪里用到了
---

# HCE 代码语义检索（渐进式披露）

后端是 **hce**（client-server + push 模式）：`hce-cli`（本机客户端，持有源码）→ `hce-server`
（切分 / embedding / 检索）→ Milvus 存向量。tree-sitter AST 切块 + 向量/keyword 混合检索。
`search` 默认先增量 sync 再检索，结果始终最新。**源码只在本机扫描，仅变更文件内容推送到服务端。**

> 本 skill 可整目录复制到任意项目的 `.claude/skills/hce/`。**唯一需要配置的是服务端地址**
> （本机 / 局域网 IP / 公网域名），见下方"配置服务端地址"。

## Quickstart（最小可用）

```bash
# 语义检索：返回相关代码片段/路径（stdout 非空即有结果）
bash ./scripts/search.sh "<你的检索问题>"
```

- 必须在**项目目录**中运行（脚本自动定位 git 根；兼容 worktree）。
- **查询用完整的一句话描述意图**（如"订单收货人手机号脱敏在哪处理"），别用单个词——语义检索对措辞敏感，完整描述召回更准。
- `HCE_TOPK` 可调返回条数（默认 10）；召回不全时可调大到 15-20。

## 配置服务端地址（复制本 skill 后的唯一必做项）

复制 `config.example.sh` 为同目录 `config.sh`，按场景填 `HCE_BASE_URL`：

| 场景 | HCE_BASE_URL 示例 |
|------|-------------------|
| 本机（docker compose 本地起栈） | 留空即可，默认 `http://localhost:9528/api/v1` |
| 局域网（连他人/服务器部署的栈） | `http://192.168.1.50:9528/api/v1` |
| 公网（已部署域名） | `https://hce.example.com/api/v1` |

> 端口约定：**9528** = nginx 前端反代（推荐走这个）；**9527** = 后端直连（调试用）。
> 也可不写 config.sh，直接 `export HCE_BASE_URL=...` 临时覆盖。

## 前置条件（每台机器一次）

1. **hce 后端可达**：本机用 `docker compose up -d` 起栈，或把 `HCE_BASE_URL` 指向已部署的局域网/公网地址。
2. **hce-cli 已编译**：`go build -o bin/hce-cli ./cmd/hce-cli`（纯客户端，**无需 CGO**）。
   脚本按 `HCE_CLI` 环境变量 > PATH > `~/Workspace/code/hce/bin/hce-cli` 顺序定位。
3. **首次建索引**：`bash ./scripts/index.sh`（= `hce-cli sync`）。之后 search 会自动增量同步。

详细部署见 `references/setup.md`。

## 【强制约束】清理/重建索引须用户同意

- **禁止擅自 `clear` 或全量重建索引**。这类操作会重新计算大量 embedding，消耗额度/token。
  必须**先经用户明确同意**才能执行 `hce-cli clear` 或全量重建。
- search 默认的**增量 sync**（仅处理变更文件，无变更时零 EMB 调用）不受此限，可正常使用。

## 输出契约（必须遵守）

1. **回显**：search 的 stdout 必须原样回显给用户。
2. **search 成功提示**：search stdout 非空时，必须额外告诉用户"已检索到结果"。
3. **失败/空结果**：按退出码与 stderr 告知用户。

## References（按需加载）

- `references/setup.md`：部署后端 + 配置服务端地址（局域网 / 公网）
- `references/contract.schema.md`：输入/输出与退出码
- `references/examples.catalog.md`：示例集合
- `references/security-privacy.md`：安全与隐私边界

# Contract：Inputs / Outputs / Exit Codes

后端为 `hce-cli`（client-server + push 模式，详见 SKILL.md）。服务端地址由 `HCE_BASE_URL` 决定。

## Search

```bash
bash /path/to/skill/scripts/search.sh "<query>"
```

**Input**
- `<query>`：自然语言查询（用完整一句话，别用单词）
- 环境变量：
  - `HCE_BASE_URL`：服务端地址（本机 / 局域网 / 公网；不设用默认 `http://localhost:9528/api/v1`）
  - `HCE_TOPK`：返回条数（默认 10）
  - `HCE_CLI`：指定 hce-cli 路径

**Output (stdout)**
- 有结果时为非空文本：`Path: <相对路径>` + 带行号的代码片段（hce-cli 的 text 格式）

**Exit codes**
- `2`：用法错误 / 缺参数
- `4`：找不到 hce-cli（未编译 / 未设 HCE_CLI）
- 非 0（来自 hce-cli）：服务端不可达或检索失败 —— 确认后端在跑、`HCE_BASE_URL` 指向正确地址

## Index

```bash
bash /path/to/skill/scripts/index.sh    # = hce-cli sync
```

- 首次全量、之后增量（size+mtime 快路径 + sha256 慢路径）；search 默认也会先 sync，通常无需手动跑。

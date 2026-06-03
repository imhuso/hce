# Contract：命令 / Inputs / Outputs / Exit Codes

后端为 `hce-cli`（client-server + push 模式，详见 SKILL.md）。服务端地址按分层配置解析（见 setup.md）。

## Search

```bash
hce-cli search "<query>" [-k 10] [-f text|json] [--no-sync]
```

**Input**
- `<query>`：自然语言查询（用完整一句话，别用单词）
- `-k`：返回条数（默认 5，建议 10；召回不全时调大到 15-20）
- `-f`：输出格式 `text`（默认）/ `json`
- `--no-sync`：跳过增量 sync，仅检索
- 环境变量 `HCE_BASE_URL` / 旗标 `--base-url`：一次性覆盖服务端地址

**Output (stdout)**
- 有结果时为非空文本：`Path: <相对路径>` + 带行号的代码片段（text 格式）

**Exit codes**
- 非 0：用法错误、`hce-cli` 不在 PATH、或后端不可达 / 检索失败
  （确认 hce-cli 已装、后端在跑、`hce-cli config` 的地址正确）

## Sync（建/增量更新索引）

```bash
hce-cli sync
```
- 首次全量、之后增量（size+mtime 快路径 + sha256 慢路径）；search 默认也会先 sync，通常无需手动跑。

## Config（查看 / 设置全局地址）

```bash
hce-cli config                                   # 查看全局地址 + 优先级
hce-cli config --base-url http://<ip>:9528/api/v1   # 写入 ~/.hce/config.json
```

## 其他

```bash
hce-cli status     # 当前 codebase 配置 / 生效 base_url / 上次 sync
hce-cli list       # 列出服务端所有已索引 collection
hce-cli clear      # 清除当前 codebase 索引（⚠ 须用户同意，见 SKILL.md 强制约束）
```

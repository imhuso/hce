# Contract：命令速查 / 退出码

服务端地址按分层配置解析（见 setup.md）；查询技巧与 `clear` 强制约束见 SKILL.md。

## 命令

```bash
hce search "<query>" [-k 5] [-f text|json] [--no-sync]  # 语义检索（默认先增量 sync）
hce sync          # 建 / 增量更新索引（首次全量，之后 size+mtime 快路径 + sha256 慢路径）
hce status        # codebase 配置 / 生效 base_url / 在线·离线 / 上次 sync
hce config [--base-url <url>]    # 查看 / 写入全局地址（~/.hce/config.json）
hce init [--id <name>]           # 显式初始化 .hce/config.json（ID 默认按项目路径派生）
hce list          # 列出服务端所有已索引 collection
hce clear         # 清除当前 codebase 索引（⚠ 须用户同意）
```

## search 参数
- `-k`：返回条数，默认 5
- `-f`：`text`（默认）/ `json`
- `--no-sync`：跳过增量 sync，仅检索
- `--base-url` / `HCE_BASE_URL`：一次性覆盖服务端地址

## 输出 / 退出码
- **stdout**：有结果时非空，`Path: <相对路径>` + 带行号代码片段（text 格式）
- **退出非 0**：用法错误 / `hce` 不在 PATH / 后端不可达 / 检索失败

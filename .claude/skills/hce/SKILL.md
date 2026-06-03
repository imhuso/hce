---
name: hce
description: |
  代码语义检索 + 首次接入引导：在当前项目用自然语言定位相关代码（实现逻辑、在哪个文件、哪里处理）；项目第一次用时引导式初始化（自检后端连通、配地址、首次索引）。
  优先用 hce-cli search 做语义检索；精确字面匹配可用 grep/rg。
  【强制】hce-cli clear / 全量重建须用户明确同意（重算 embedding 费 token）；增量 sync 不限。
  触发词：哪里、在哪、代码在哪、怎么实现的、哪个文件、哪里处理、哪里定义、怎么写的、哪里用到了；初始化、接入、配置 hce、启用 hce、第一次用、连不上、后端地址
---

# HCE 代码语义检索

在项目目录内执行（自动识别项目根；默认先增量 sync 再检索，结果最新）：

```bash
hce-cli search "<完整一句话描述意图>" -k 10
```

- 查询用**完整的一句话**（如"订单收货人手机号脱敏在哪处理"），别用单词——召回更准。
- 召回不全时把 `-k` 调大到 15-20。

## 首次接入（引导式初始化）

新项目第一次用，或用户说"初始化 / 接入 / 配置 hce / 连不上"时，按序引导。
**无需手动建任何文件**——`.hce/config.json` 在任一命令首次运行时自动生成，`codebase_id` 默认从项目绝对路径派生。

1. **查 CLI**：`hce-cli version`。报错=不在 PATH → 见 `references/setup.md` 安装。
2. **一键自检**：`hce-cli status`。一条命令给出 项目根 / codebase_id / base_url **+ 在线·离线** / 已索引文件数 / 上次 sync。据此分支：
   - 显示**离线** → 后端不可达。先问用户后端在哪（本机 docker / 局域网 IP / 公网域名），再 `hce-cli config --base-url <url>` 写全局默认；个别项目要连别的后端，则在该项目里加 `--base-url` 或写进项目 `.hce/config.json`。默认地址是 `localhost:9528`，**别默认用户已起本机栈，不确定就问**。
   - **在线**但已索引文件=0 / 无上次 sync → 首次库，进 4。
   - 在线且已有索引 → 已就绪，直接 search 即可。
3. **（可选）自定义 ID**：要自定义 codebase_id 用 `hce-cli init --id <name>`（默认派生的 ID 已稳定唯一，一般不用动）。
4. **首次索引**：`hce-cli sync`（首次全量 embedding，之后增量；增量 sync 无需额外确认，全量重建 / clear 才需）。
5. **报告就绪**：回显 codebase_id、生效后端地址、已索引文件数，然后正常 `search`。

## 输出契约
1. `search` 的 stdout **原样回显**给用户。
2. stdout 非空 → 额外说明"已检索到结果"。
3. 报错（多为 hce-cli 不在 PATH，或后端不可达）→ 据 stderr 告知用户，排查见 `references/setup.md`。

## 强制约束
**禁止擅自 `hce-cli clear` 或全量重建索引**（重算大量 embedding，费额度/token），须先经用户同意。

## References（按需加载）
- `setup.md`：装 hce-cli（单一跨平台二进制）+ 配服务端地址（局域网 / 公网，分层 JSON）+ 连通性自检
- `contract.schema.md`：命令 / 退出码
- `examples.catalog.md`：查询示例
- `security-privacy.md`：隐私边界

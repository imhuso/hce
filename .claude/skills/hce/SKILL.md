---
name: hce
description: |
  代码语义检索 + 首次接入引导：在当前项目用自然语言定位代码（实现逻辑 / 在哪个文件 / 哪里处理）；首次使用时引导初始化（自检连通、配地址、首次索引）。
  优先用 hce search；精确字面匹配用 grep/rg。【强制】hce clear / 全量重建须用户明确同意（重算 embedding 费 token），增量 sync 不限。
  触发词：哪里、在哪、怎么实现的、哪个文件、哪里处理、哪里定义、哪里用到了；初始化、接入、配置 hce、连不上、后端地址
---

# HCE 代码语义检索

项目目录内执行（自动找项目根，默认先增量 sync 再检索）：

```bash
hce search "<完整一句话描述意图>" -k 10
```

- 查询用**完整一句话**（如"订单收货人手机号脱敏在哪处理"），别用单词，召回更准；不全就把 `-k` 调到 15-20。
- stdout **原样回显**给用户；报错（多为 hce 不在 PATH 或后端不可达）据 stderr 告知，排查见 `references/setup.md`。

## 首次接入

新项目第一次用，或用户说"初始化 / 接入 / 连不上"时，跑 `hce status` 自检——输出 项目根 / codebase_id / base_url **+ 在线·离线** / 已索引数（`.hce/config.json` 首次运行自动生成，无需手动建）：

- **离线** → **先问用户后端在哪**（本机 docker / 局域网 IP / 公网域名），别默认其已起本机栈；再 `hce config --base-url <url>`。装 CLI / 配地址见 `references/setup.md`。
- **在线无索引** → `hce sync` 首次索引后再 search。
- **在线有索引** → 直接 search。

## References（按需加载）
- `setup.md`：装 hce + 配服务端地址（分层 JSON）+ 连通性自检
- `contract.schema.md`：命令速查 / 退出码
- `examples.catalog.md`：查询示例
- `security-privacy.md`：隐私边界

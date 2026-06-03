---
name: hce
description: |
  代码语义检索：在当前项目用自然语言定位相关代码（实现逻辑、在哪个文件、哪里处理）。
  优先用 hce-cli search 做语义检索；精确字面匹配可用 grep/rg。
  【强制】hce-cli clear / 全量重建须用户明确同意（重算 embedding 费 token）；增量 sync 不限。
  触发词：哪里、在哪、代码在哪、怎么实现的、哪个文件、哪里处理、哪里定义、怎么写的、哪里用到了
---

# HCE 代码语义检索

在项目目录内执行（自动识别项目根；默认先增量 sync 再检索，结果最新）：

```bash
hce-cli search "<完整一句话描述意图>" -k 10
```

- 查询用**完整的一句话**（如"订单收货人手机号脱敏在哪处理"），别用单词——召回更准。
- 召回不全时把 `-k` 调大到 15-20。

## 输出契约
1. `search` 的 stdout **原样回显**给用户。
2. stdout 非空 → 额外说明"已检索到结果"。
3. 报错（多为 hce-cli 不在 PATH，或后端不可达）→ 据 stderr 告知用户，排查见 `references/setup.md`。

## 强制约束
**禁止擅自 `hce-cli clear` 或全量重建索引**（重算大量 embedding，费额度/token），须先经用户同意。

## References（按需加载）
- `setup.md`：装 hce-cli（单一跨平台二进制）+ 配服务端地址（局域网 / 公网，分层 JSON）
- `contract.schema.md`：命令 / 退出码
- `examples.catalog.md`：查询示例
- `security-privacy.md`：隐私边界

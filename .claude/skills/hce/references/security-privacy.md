# Security & Privacy

- **源码不出本机文件系统**：hce 是 push 模式，`hce-cli` 在本机扫描，只把**变更文件内容**发到 hce-server；服务端从不读你的磁盘。
- **embedding 会把代码切块发给所选供应商**（gemini / openai / voyageai / ollama）计算向量。
  用 ollama 或 LM Studio 等自托管模型可做到全程不出内网。供应商凭据配置在**服务端** `.env`（已 gitignore），不要提交。
- **连公网后端时**：`HCE_BASE_URL` 指向公网域名意味着代码内容经公网传给该后端，务必确认后端可信、链路用 HTTPS、并对端点加鉴权。
- **客户端落地文件**：hce-cli 在被检索的项目根写 `.hce/`（codebase 配置与本地 state）。请在该项目 `.gitignore` 忽略 `.hce/`。
- **config.sh 不入库**：本 skill 的 `config.sh` 已在仓库 `.gitignore`；它仅含 URL/路径，无密钥。
- **输出克制**：不要把代码中的敏感信息回显到用户可见输出；检索结果只保留相关、精简的片段。

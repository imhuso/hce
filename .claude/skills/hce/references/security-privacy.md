# Security & Privacy

- **源码不出本机**：push 模式，`hce-cli` 在本机扫描，只把变更文件内容发到 hce-server，服务端不读你的磁盘。
- **embedding 会把代码切块发给所选供应商**计算向量（用 ollama / LM Studio 等自托管模型可全程不出内网）。供应商凭据配在**服务端** `.env`（gitignore），勿提交。
- **连公网后端**意味着代码内容经公网传输——确认后端可信、走 HTTPS、端点加鉴权。
- **客户端落地**：`hce-cli` 在项目根写 `.hce/`（codebase 配置 + 本地 state），在项目 `.gitignore` 忽略 `.hce/`。全局地址存 `~/.hce/config.json`，仅含 URL。
- **输出克制**：检索结果只保留相关、精简的片段，别把敏感信息回显到用户可见输出。

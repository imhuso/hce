# Examples

```bash
hce-cli search "订单收货人手机号脱敏在哪里处理" -k 10
hce-cli search "JWT token 校验拦截器的实现"
hce-cli search "用户登录鉴权逻辑在哪个文件" -k 15
hce-cli search "where the order number uniqueness is validated" -f json

# 一次性切换后端（不改全局配置）
HCE_BASE_URL="https://hce.example.com/api/v1" hce-cli search "支付回调验签"
```

# Examples

## Search

```bash
bash ./scripts/search.sh "订单收货人手机号脱敏在哪里处理"
bash ./scripts/search.sh "JWT token 校验拦截器的实现"
bash ./scripts/search.sh "用户登录鉴权逻辑在哪个文件"
HCE_TOPK=20 bash ./scripts/search.sh "where the order number uniqueness is validated"
```

## Index

```bash
bash ./scripts/index.sh    # 建立/增量更新索引（= hce-cli sync）
```

## 切换服务端地址

```bash
# 临时连局域网部署的后端
HCE_BASE_URL="http://192.168.1.50:9528/api/v1" bash ./scripts/search.sh "购物车合并逻辑"

# 临时连公网部署
HCE_BASE_URL="https://hce.example.com/api/v1" bash ./scripts/search.sh "支付回调验签"
```

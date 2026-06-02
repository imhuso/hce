# 构建阶段
FROM node:22-alpine AS builder
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
# 容器内通过 nginx 反代到后端，前端始终使用同源 /api/v1
ENV VITE_API_BASE=/api/v1
RUN npm run build

# 运行阶段：nginx 静态托管 + /api 反代到后端
FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

# ============================================================
# 第一阶段：编译 Go 二进制
# ============================================================
FROM golang:1.26-alpine AS builder

WORKDIR /app

# 先复制依赖文件，利用 Docker 层缓存加速构建
# 如果 go.mod/go.sum 没变，docker build 会复用这一层
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码并编译
COPY . .
# CGO_ENABLED=0: 禁用 CGO，编译纯静态二进制（不依赖 libc）
# GOOS=linux: 目标操作系统
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/rag-server ./cmd/server

# ============================================================
# 第二阶段：最小运行镜像
# ============================================================
FROM alpine:3.21

# 安装 CA 证书（HTTPS 请求需要）和时区数据
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/bin/rag-server .

# 复制前端静态文件和数据库迁移脚本
COPY --from=builder /app/web ./web
COPY --from=builder /app/migrations ./migrations

# 暴露端口（仅文档作用，实际端口由 config.yaml 控制）
EXPOSE 8080

# 启动应用
# 配置文件和密钥通过 docker-compose volumes 和 environment 注入
CMD ["./rag-server"]
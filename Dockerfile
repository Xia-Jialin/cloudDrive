# --- 构建阶段 ---
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/main.go

# --- 运行阶段 ---
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/server ./server
COPY configs ./configs
# 创建 uploads 目录用于持久化存储
RUN mkdir -p /app/uploads
VOLUME ["/app/uploads"]
EXPOSE 8080
# 支持通过环境变量覆盖配置路径
ENV CONFIG_PATH=./configs/config.yaml
CMD ./server -config "$CONFIG_PATH" 
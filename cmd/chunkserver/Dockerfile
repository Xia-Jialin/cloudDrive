FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装依赖
RUN apk add --no-cache git

# 复制go.mod和go.sum
COPY go.mod go.sum ./
# RUN go mod download

# 复制源代码
COPY . .

# 编译块存储服务
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o chunkserver cmd/chunkserver/main.go

# 使用轻量级的alpine镜像
FROM alpine:latest

# 安装必要的运行时依赖
RUN apk --no-cache add ca-certificates tzdata

# 设置工作目录
WORKDIR /app

# 从builder阶段复制编译好的二进制文件
COPY --from=builder /app/chunkserver /app/
COPY --from=builder /app/configs/chunkserver.yaml /app/configs/

# 创建上传目录
RUN mkdir -p /app/uploads

# 暴露HTTP和gRPC端口
EXPOSE 8081 9000

# 设置环境变量
ENV ETCD_ENDPOINT="etcd:2379"
ENV ETCD_KEY="/clouddrive/chunkserver/config"

# 启动命令
CMD ["/app/chunkserver", "--config", "/app/configs/chunkserver.yaml"] 
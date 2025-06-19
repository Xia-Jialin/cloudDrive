FROM golang:1.18-alpine AS builder

WORKDIR /app

# 复制go.mod和go.sum
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o chunkserver ./cmd/chunkserver

FROM alpine:latest

WORKDIR /app

# 安装基础工具
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
ENV TZ=Asia/Shanghai

# 从builder阶段复制编译好的应用
COPY --from=builder /app/chunkserver .
COPY --from=builder /app/configs/chunkserver.yaml ./configs/chunkserver.yaml

# 创建上传目录
RUN mkdir -p /app/uploads/multipart

# 暴露端口
EXPOSE 8081 9000

# 运行应用
CMD ["./chunkserver"] 
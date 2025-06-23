#!/bin/bash

# CloudDrive 一键简化部署脚本
# 作者: AI Assistant
# 版本: v1.1
# 用法: ./deploy-simple.sh [选项]
# 生产环境默认使用MinIO作为存储后端

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置参数
NAMESPACE=${NAMESPACE:-default}
DEPLOYMENT_TYPE=${DEPLOYMENT_TYPE:-docker-compose}
ENABLE_MONITORING=${ENABLE_MONITORING:-false}
SKIP_BUILD=${SKIP_BUILD:-false}
CLEAN_ONLY=${CLEAN_ONLY:-false}

# 打印帮助信息
print_help() {
    echo -e "${BLUE}CloudDrive 一键简化部署脚本${NC}"
    echo "=================================="
    echo ""
    echo "用法: ./deploy-simple.sh [选项]"
    echo ""
    echo "选项:"
    echo "  -t, --type TYPE        部署类型 (docker-compose|k8s|k8s-simple) [默认: docker-compose]"
    echo "  -n, --namespace NS     Kubernetes命名空间 [默认: default]"
    echo "  -m, --monitoring       启用监控功能"
    echo "  -s, --skip-build       跳过镜像构建"
    echo "  -c, --clean            清理现有部署"
    echo "  -h, --help             显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  ./deploy-simple.sh                           # Docker Compose部署"
    echo "  ./deploy-simple.sh -t k8s -m                 # K8s部署+监控"
    echo "  ./deploy-simple.sh -t k8s-simple             # K8s简化部署"
    echo "  ./deploy-simple.sh -c                        # 清理部署"
    echo ""
    echo "注意:"
    echo "  • 生产环境默认使用MinIO作为存储后端"
    echo "  • 自动创建clouddrive存储桶"
    echo "  • MinIO默认账号: minioadmin/minioadmin"
    echo "  • API通过前端nginx代理访问，支持多Pod负载均衡"
}

# 打印状态信息
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查依赖
check_dependencies() {
    print_status "检查依赖..."
    
    case $DEPLOYMENT_TYPE in
        "docker-compose")
            if ! command -v docker &> /dev/null; then
                print_error "Docker 未安装"
                exit 1
            fi
            if ! command -v docker-compose &> /dev/null; then
                print_error "Docker Compose 未安装"
                exit 1
            fi
            ;;
        "k8s"|"k8s-simple")
            if ! command -v kubectl &> /dev/null; then
                print_error "kubectl 未安装"
                exit 1
            fi
            # 检查k8s连接
            if ! kubectl cluster-info &> /dev/null; then
                print_error "无法连接到Kubernetes集群"
                exit 1
            fi
            ;;
    esac
}

# 构建镜像
build_images() {
    if [ "$SKIP_BUILD" = "true" ]; then
        print_status "跳过镜像构建"
        return
    fi
    
    print_status "构建Docker镜像..."
    
    # 并行构建所有镜像
    {
        docker build -t registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive:latest . &
        docker build -t registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver:latest -f cmd/chunkserver/Dockerfile . &
        docker build -t registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_web:latest -f web/Dockerfile web/ &
        wait
    }
    
    print_status "镜像构建完成"
}

# Docker Compose 部署
deploy_docker_compose() {
    print_status "使用 Docker Compose 部署..."
    
    # 创建简化的docker-compose配置
    cat > docker-compose.simple.yml << 'EOF'
version: '3.8'
services:
  mysql:
    image: mysql:8.0
    container_name: clouddrive-mysql
    restart: always
    environment:
      MYSQL_ROOT_PASSWORD: 123456
      MYSQL_DATABASE: clouddrive
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      timeout: 20s
      retries: 10

  redis:
    image: redis:7.2-alpine
    container_name: clouddrive-redis
    restart: always
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  etcd:
    image: quay.io/coreos/etcd:v3.5.0
    container_name: clouddrive-etcd
    restart: always
    ports:
      - "2379:2379"
    environment:
      - ETCD_ADVERTISE_CLIENT_URLS=http://0.0.0.0:2379
      - ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379
      - ETCD_LISTEN_PEER_URLS=http://0.0.0.0:2380
      - ETCD_INITIAL_ADVERTISE_PEER_URLS=http://etcd:2380
      - ALLOW_NONE_AUTHENTICATION=yes
      - ETCD_INITIAL_CLUSTER=node1=http://etcd:2380
      - ETCD_NAME=node1
      - ETCD_DATA_DIR=/etcd-data
    volumes:
      - etcd_data:/etcd-data

  minio:
    image: minio/minio:latest
    container_name: clouddrive-minio
    restart: always
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      - MINIO_ACCESS_KEY=minioadmin
      - MINIO_SECRET_KEY=minioadmin
    volumes:
      - minio_data:/data
    command: server /data --console-address ":9001"

  chunkserver:
    image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver:latest
    container_name: clouddrive-chunkserver
    restart: always
    depends_on:
      redis:
        condition: service_healthy
      etcd:
        condition: service_started
      minio:
        condition: service_started
    ports:
      - "8081:8081"
      - "9000:9000"
    volumes:
      - chunkserver_data:/app/uploads
      - ./configs:/app/configs
    environment:
      - ETCD_ENDPOINT=etcd:2379

  api-server:
    image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive:latest
    container_name: clouddrive-api-server
    restart: always
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy
      etcd:
        condition: service_started
      chunkserver:
        condition: service_started
    ports:
      - "8080:8080"
    volumes:
      - ./configs:/app/configs
      - api_logs:/app/logs
    environment:
      - ETCD_ENDPOINT=etcd:2379

  web:
    image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_web:latest
    container_name: clouddrive-web
    restart: always
    depends_on:
      - api-server
    ports:
      - "80:80"
    volumes:
      - ./web-config/nginx.conf:/etc/nginx/nginx.conf:ro

volumes:
  mysql_data:
  redis_data:
  etcd_data:
  minio_data:
  chunkserver_data:
  api_logs:
EOF

    # 创建Docker Compose环境的nginx配置
    mkdir -p ./web-config
    cat > ./web-config/nginx.conf << 'NGINX_EOF'
worker_processes 1;
events { worker_connections 1024; }
http {
  include       mime.types;
  default_type  application/octet-stream;
  sendfile        on;
  keepalive_timeout  65;

  server {
    listen 80;
    server_name localhost;
    root /usr/share/nginx/html;
    index index.html;

    location /api/ {
      proxy_pass http://api-server:8080;
      proxy_set_header Host $host;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_connect_timeout 30s;
      proxy_send_timeout 30s;
      proxy_read_timeout 30s;
    }

    location / {
      try_files $uri $uri/ /index.html;
    }
  }
}
NGINX_EOF

    # 启动服务
    docker-compose -f docker-compose.simple.yml up -d
    
    # 等待服务启动
    print_status "等待服务启动..."
    sleep 30
    
    # 初始化MinIO bucket
    print_status "初始化MinIO bucket..."
    NETWORK_NAME=$(docker-compose -f docker-compose.simple.yml config --services | head -1 | xargs -I {} docker-compose -f docker-compose.simple.yml ps -q {} | head -1 | xargs docker inspect --format='{{range .NetworkSettings.Networks}}{{.NetworkID}}{{end}}' | head -c 12)
    docker run --rm --network ${NETWORK_NAME}_default minio/mc:latest /bin/sh -c "
        mc alias set myminio http://minio:9000 minioadmin minioadmin &&
        mc mb myminio/clouddrive --ignore-existing
    " 2>/dev/null || docker run --rm --network clouddrive_default minio/mc:latest /bin/sh -c "
        mc alias set myminio http://minio:9000 minioadmin minioadmin &&
        mc mb myminio/clouddrive --ignore-existing
    " || true
    
    # 上传配置到etcd
    print_status "上传配置到etcd..."
    sleep 10  # 等待etcd完全启动
    ETCD_ENDPOINT="localhost:2379" ./scripts/upload_config_to_etcd.sh || true
    
    print_status "Docker Compose 部署完成！"
    echo ""
    echo "访问地址:"
    echo "  • 前端: http://localhost"
    echo "  • API: http://localhost/api/ (通过前端代理)"
    echo "  • MinIO控制台: http://localhost:9001 (minioadmin/minioadmin)"
}

# Kubernetes 简化部署
deploy_k8s_simple() {
    print_status "使用 Kubernetes 简化部署..."
    
    # 创建命名空间
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # 创建一体化部署配置
    cat > clouddrive-all-in-one.yaml << 'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: clouddrive-config
  namespace: default
data:
  config.yaml: |
    database:
      user: root
      password: "123456"
      host: mysql
      port: 3306
      name: clouddrive
      charset: utf8mb4
      parseTime: true
      loc: Local
    redis:
      addr: "redis:6379"
      password: ""
      db: 0
      pool_size: 10
    storage:
      type: minio
      local_dir: uploads
      chunk_server:
        enabled: true
        url: "http://chunkserver:8081"
        temp_dir: "/tmp/chunk_client"
      minio:
        endpoint: "minio:9000"
        access_key: "minioadmin"
        secret_key: "minioadmin"
        bucket: "clouddrive"
        use_ssl: false
    environment: "production"
    monitoring:
      log_level: "info"
      metrics_enabled: true
  chunkserver.yaml: |
    server:
      grpc_port: 9000
      http_port: 8081
      upload_max_size: 1073741824
    redis:
      addr: "redis:6379"
      user: ""
      password: ""
      db: 0
      pool_size: 10
    storage:
      type: "minio"
      local_dir: "./uploads"
      minio:
        endpoint: "minio:9000"
        access_key: "minioadmin"
        secret_key: "minioadmin"
        bucket: "clouddrive"
        use_ssl: false
    security:
      jwt_secret: "your-super-secret-key-for-jwt-token-signing"
    environment: "production"
  nginx.conf: |
    worker_processes 1;
    events { worker_connections 1024; }
    http {
      include       mime.types;
      default_type  application/octet-stream;
      sendfile        on;
      keepalive_timeout  65;

      server {
        listen 80;
        server_name localhost;
        root /usr/share/nginx/html;
        index index.html;

        location /api/ {
          proxy_pass http://api-server:8080;
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          proxy_connect_timeout 30s;
          proxy_send_timeout 30s;
          proxy_read_timeout 30s;
        }

        location / {
          try_files $uri $uri/ /index.html;
        }
      }
    }

---
# MySQL
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        env:
        - name: MYSQL_ROOT_PASSWORD
          value: "123456"
        - name: MYSQL_DATABASE
          value: "clouddrive"
        ports:
        - containerPort: 3306
        volumeMounts:
        - name: mysql-data
          mountPath: /var/lib/mysql
      volumes:
      - name: mysql-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: mysql
  namespace: default
spec:
  selector:
    app: mysql
  ports:
  - port: 3306

---
# Redis
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7.2-alpine
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-data
          mountPath: /data
      volumes:
      - name: redis-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: default
spec:
  selector:
    app: redis
  ports:
  - port: 6379

---
# etcd
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: etcd
  template:
    metadata:
      labels:
        app: etcd
    spec:
      containers:
      - name: etcd
        image: quay.io/coreos/etcd:v3.5.0
        ports:
        - containerPort: 2379
        env:
        - name: ETCD_ADVERTISE_CLIENT_URLS
          value: "http://0.0.0.0:2379"
        - name: ETCD_LISTEN_CLIENT_URLS
          value: "http://0.0.0.0:2379"
        - name: ETCD_LISTEN_PEER_URLS
          value: "http://0.0.0.0:2380"
        - name: ETCD_INITIAL_ADVERTISE_PEER_URLS
          value: "http://etcd:2380"
        - name: ALLOW_NONE_AUTHENTICATION
          value: "yes"
        - name: ETCD_INITIAL_CLUSTER
          value: "node1=http://etcd:2380"
        - name: ETCD_NAME
          value: "node1"
        volumeMounts:
        - name: etcd-data
          mountPath: /etcd-data
      volumes:
      - name: etcd-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: etcd
  namespace: default
spec:
  selector:
    app: etcd
  ports:
  - port: 2379

---
# MinIO
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: minio/minio:latest
        ports:
        - containerPort: 9000
        - containerPort: 9001
        env:
        - name: MINIO_ACCESS_KEY
          value: "minioadmin"
        - name: MINIO_SECRET_KEY
          value: "minioadmin"
        args:
        - server
        - /data
        - --console-address
        - ":9001"
        volumeMounts:
        - name: minio-data
          mountPath: /data
      volumes:
      - name: minio-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: default
spec:
  selector:
    app: minio
  ports:
  - port: 9000
    name: api
  - port: 9001
    name: console

---
# ChunkServer
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chunkserver
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: chunkserver
  template:
    metadata:
      labels:
        app: chunkserver
    spec:
      containers:
      - name: chunkserver
        image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver:latest
        ports:
        - containerPort: 8081
        - containerPort: 9000
        env:
        - name: ETCD_ENDPOINT
          value: "etcd:2379"
        volumeMounts:
        - name: chunk-data
          mountPath: /app/uploads
        - name: config
          mountPath: /app/configs
      volumes:
      - name: chunk-data
        emptyDir: {}
      - name: config
        configMap:
          name: clouddrive-config
---
apiVersion: v1
kind: Service
metadata:
  name: chunkserver
  namespace: default
spec:
  selector:
    app: chunkserver
  ports:
  - port: 8081
    name: http
  - port: 9000
    name: grpc

---
# API Server
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
    spec:
      containers:
      - name: api-server
        image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive:latest
        ports:
        - containerPort: 8080
        env:
        - name: ETCD_ENDPOINT
          value: "etcd:2379"
        volumeMounts:
        - name: config
          mountPath: /app/configs
        livenessProbe:
          httpGet:
            path: /api/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: clouddrive-config
---
apiVersion: v1
kind: Service
metadata:
  name: api-server
  namespace: default
spec:
  selector:
    app: api-server
  ports:
  - port: 8080
    targetPort: 8080

---
# Web Frontend
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: web
        image: registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_web:latest
        ports:
        - containerPort: 80
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: empty-conf
          mountPath: /etc/nginx/conf.d
      volumes:
      - name: nginx-config
        configMap:
          name: clouddrive-config
      - name: empty-conf
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: default
spec:
  type: NodePort
  selector:
    app: web
  ports:
  - port: 80
    targetPort: 80
    nodePort: 30081

---
apiVersion: v1
kind: Service
metadata:
  name: minio-nodeport
  namespace: default
spec:
  type: NodePort
  selector:
    app: minio
  ports:
  - port: 9001
    targetPort: 9001
    nodePort: 30091
    name: console
EOF

    # 部署到k8s
    kubectl apply -f clouddrive-all-in-one.yaml -n $NAMESPACE
    
    # 等待部署完成
    print_status "等待Pod启动..."
    kubectl wait --for=condition=ready pod -l app=mysql -n $NAMESPACE --timeout=300s || true
    kubectl wait --for=condition=ready pod -l app=redis -n $NAMESPACE --timeout=300s || true
    kubectl wait --for=condition=ready pod -l app=etcd -n $NAMESPACE --timeout=300s || true
    kubectl wait --for=condition=ready pod -l app=minio -n $NAMESPACE --timeout=300s || true
    kubectl wait --for=condition=ready pod -l app=chunkserver -n $NAMESPACE --timeout=300s || true
    kubectl wait --for=condition=ready pod -l app=api-server -n $NAMESPACE --timeout=300s || true
    
    # 初始化MinIO bucket
    print_status "初始化MinIO bucket..."
    kubectl run minio-init --image=minio/mc:latest --rm -i --restart=Never -n $NAMESPACE -- /bin/sh -c "
        mc alias set myminio http://minio:9000 minioadmin minioadmin &&
        mc mb myminio/clouddrive --ignore-existing
    " || true
    
    # 上传配置
    print_status "上传配置到etcd..."
    sleep 10
    kubectl run config-uploader --image=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive:latest --rm -i --restart=Never -n $NAMESPACE -- /bin/sh -c "
        cd /app && 
        go run scripts/config_to_etcd.go --config configs/config.yaml --etcd etcd:2379 --key /clouddrive/server/config &&
        go run scripts/config_to_etcd.go --config configs/chunkserver.yaml --etcd etcd:2379 --key /clouddrive/chunkserver/config
    " || true
    
    # 获取访问信息
    NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}' || kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
    
    print_status "Kubernetes 简化部署完成！"
    echo ""
    echo "访问地址:"
    echo "  • 前端: http://$NODE_IP:30081"
    echo "  • API: http://$NODE_IP:30081/api/ (通过前端代理)"
    echo "  • MinIO控制台: http://$NODE_IP:30091 (minioadmin/minioadmin)"
    
    # 清理临时文件
    rm -f clouddrive-all-in-one.yaml
}

# 完整的k8s部署（使用现有配置）
deploy_k8s_full() {
    print_status "使用完整 Kubernetes 部署..."
    
    if [ "$ENABLE_MONITORING" = "true" ]; then
        make deploy-all-enhanced
    else
        make deploy-all
    fi
}

# 清理部署
clean_deployment() {
    print_status "清理现有部署..."
    
    case $DEPLOYMENT_TYPE in
        "docker-compose")
            docker-compose -f docker-compose.simple.yml down -v 2>/dev/null || true
            docker-compose down -v 2>/dev/null || true
            rm -f docker-compose.simple.yml
            rm -rf ./web-config
            ;;
        "k8s"|"k8s-simple")
            # 删除部署文件（如果存在）
            kubectl delete -f clouddrive-all-in-one.yaml -n $NAMESPACE 2>/dev/null || true
            
            # 直接删除所有相关资源
            print_status "删除所有CloudDrive相关资源..."
            kubectl delete deployment mysql redis etcd minio chunkserver api-server web -n $NAMESPACE 2>/dev/null || true
            kubectl delete service mysql redis etcd minio chunkserver api-server web web-nodeport api-server-nodeport minio-nodeport -n $NAMESPACE 2>/dev/null || true
            kubectl delete configmap clouddrive-config -n $NAMESPACE 2>/dev/null || true
            kubectl delete pvc --all -n $NAMESPACE 2>/dev/null || true
            
            # 清理可能的遗留资源（避免循环调用）
            if [ -f Makefile.old ]; then
                make -f Makefile.old delete-all-enhanced 2>/dev/null || true
                make -f Makefile.old delete-test-env 2>/dev/null || true
            fi
            
            # 等待资源完全删除
            print_status "等待资源清理完成..."
            sleep 5
            ;;
    esac
    
    print_status "清理完成"
}

# 主函数
main() {
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -t|--type)
                DEPLOYMENT_TYPE="$2"
                shift 2
                ;;
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            -m|--monitoring)
                ENABLE_MONITORING="true"
                shift
                ;;
            -s|--skip-build)
                SKIP_BUILD="true"
                shift
                ;;
            -c|--clean)
                CLEAN_ONLY="true"
                shift
                ;;
            -h|--help)
                print_help
                exit 0
                ;;
            *)
                print_error "未知参数: $1"
                print_help
                exit 1
                ;;
        esac
    done
    
    # 如果只是清理，则执行清理后退出
    if [ "$CLEAN_ONLY" = "true" ]; then
        clean_deployment
        exit 0
    fi
    
    # 验证部署类型
    case $DEPLOYMENT_TYPE in
        "docker-compose"|"k8s"|"k8s-simple")
            ;;
        *)
            print_error "无效的部署类型: $DEPLOYMENT_TYPE"
            print_help
            exit 1
            ;;
    esac
    
    print_status "开始 CloudDrive 简化部署"
    print_status "部署类型: $DEPLOYMENT_TYPE"
    print_status "命名空间: $NAMESPACE"
    print_status "监控功能: $ENABLE_MONITORING"
    print_status "跳过构建: $SKIP_BUILD"
    echo ""
    
    # 检查依赖
    check_dependencies
    
    # 构建镜像
    build_images
    
    # 根据类型部署
    case $DEPLOYMENT_TYPE in
        "docker-compose")
            deploy_docker_compose
            ;;
        "k8s-simple")
            deploy_k8s_simple
            ;;
        "k8s")
            deploy_k8s_full
            ;;
    esac
    
    print_status "部署完成！"
    echo ""
    echo "提示:"
    echo "  • 使用 './deploy-simple.sh -c' 清理部署"
    echo "  • 使用 './deploy-simple.sh -h' 查看帮助"
}

# 执行主函数
main "$@" 
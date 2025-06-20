#!/bin/bash

# 确保脚本在出错时退出
set -e

echo "===== 开始部署 CloudDrive 服务（使用 Ingress）====="

# 检查 kubectl 是否可用
if ! command -v kubectl &> /dev/null; then
    echo "错误: kubectl 命令未找到，请安装 kubectl"
    exit 1
fi

# 检查 Ingress 控制器是否已安装
if ! kubectl get namespace ingress-nginx &> /dev/null; then
    echo "警告: 未检测到 ingress-nginx 命名空间，Ingress 控制器可能未安装"
    read -p "是否安装 Ingress 控制器? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "部署已取消"
        exit 1
    fi
    
    echo "正在安装 Nginx Ingress Controller..."
    make install-ingress-controller
    
    echo "等待 Ingress Controller 就绪..."
    kubectl wait --namespace ingress-nginx \
      --for=condition=ready pod \
      --selector=app.kubernetes.io/component=controller \
      --timeout=120s || echo "Ingress Controller 可能尚未就绪，继续部署..."
fi

# 部署基础服务
echo "正在部署基础服务 (MySQL, Redis, etcd, MinIO)..."
make deploy-test-env

# 等待基础服务就绪
echo "等待基础服务就绪..."
# 使用正确的标签
kubectl wait --for=condition=ready pod -l app=mysql-test --timeout=120s || echo "MySQL 可能尚未就绪，继续部署..."
kubectl wait --for=condition=ready pod -l app=redis-test --timeout=120s || echo "Redis 可能尚未就绪，继续部署..."
kubectl wait --for=condition=ready pod -l app=etcd --timeout=120s || echo "etcd 可能尚未就绪，继续部署..."
kubectl wait --for=condition=ready pod -l app=minio-test --timeout=120s || echo "MinIO 可能尚未就绪，继续部署..."

# 上传配置到 etcd
echo "正在上传配置到 etcd..."
make upload-config-to-etcd-k8s

# 部署应用服务
echo "正在部署 API 服务器..."
make deploy-api-server

echo "正在部署块存储服务器..."
make deploy-chunkserver

# 等待应用服务就绪
echo "等待应用服务就绪..."
kubectl wait --for=condition=ready pod -l app=api-server --timeout=120s || echo "API 服务器可能尚未就绪，继续部署..."
kubectl wait --for=condition=ready pod -l app=chunkserver --timeout=120s || echo "块存储服务器可能尚未就绪，继续部署..."

# 部署 Ingress
echo "正在部署 Ingress 规则..."
make deploy-ingress

# 等待 Ingress 就绪
echo "等待 Ingress 就绪..."
sleep 10

# 获取 Ingress 控制器的 IP 地址
echo "正在获取 Ingress 控制器的 IP 地址..."
INGRESS_IP=$(kubectl get service -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "未找到")

if [ "$INGRESS_IP" = "未找到" ]; then
    INGRESS_IP=$(kubectl get service -n ingress-nginx ingress-nginx-controller -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || echo "未找到")
fi

if [ "$INGRESS_IP" = "未找到" ]; then
    # 如果没有 LoadBalancer，尝试获取 NodePort 的地址
    INGRESS_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}' 2>/dev/null || echo "未找到")
    
    if [ "$INGRESS_IP" = "未找到" ]; then
        INGRESS_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || echo "未找到")
    fi
    
    # 获取 NodePort
    INGRESS_HTTP_PORT=$(kubectl get service -n ingress-nginx ingress-nginx-controller -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}' 2>/dev/null || echo "30080")
    INGRESS_HTTPS_PORT=$(kubectl get service -n ingress-nginx ingress-nginx-controller -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}' 2>/dev/null || echo "30443")
    
    echo "Ingress NodePort HTTP端口: $INGRESS_HTTP_PORT"
    echo "Ingress NodePort HTTPS端口: $INGRESS_HTTPS_PORT"
    
    echo
    echo "您可以通过以下地址访问服务:"
    echo "- API 服务: http://$INGRESS_IP:$INGRESS_HTTP_PORT/api"
    echo "- gRPC 服务: $INGRESS_IP:$INGRESS_HTTP_PORT (使用gRPC客户端)"
    echo
    echo "或者，您可以在hosts文件中添加以下条目:"
    echo "$INGRESS_IP clouddrive.local grpc.clouddrive.local"
    echo "然后通过以下地址访问服务:"
    echo "- API 服务: http://clouddrive.local:$INGRESS_HTTP_PORT/api"
    echo "- gRPC 服务: grpc.clouddrive.local:$INGRESS_HTTP_PORT"
    
    echo
    echo "前端开发环境可以使用以下命令启动:"
    echo "VITE_API_BASE_URL=http://$INGRESS_IP:$INGRESS_HTTP_PORT npm run dev"
    echo "或"
    echo "VITE_API_BASE_URL=http://clouddrive.local:$INGRESS_HTTP_PORT npm run dev"
    
    exit 0
fi

echo "===== CloudDrive 服务部署完成 ====="
echo
echo "Ingress 控制器 IP: $INGRESS_IP"
echo
echo "请在您的 hosts 文件中添加以下条目:"
echo "$INGRESS_IP clouddrive.local grpc.clouddrive.local"
echo
echo "然后，您可以通过以下地址访问服务:"
echo "- API 服务: http://clouddrive.local/api"
echo "- gRPC 服务: grpc.clouddrive.local:80"
echo
echo "前端开发环境可以使用以下命令启动:"
echo "VITE_API_BASE_URL=http://clouddrive.local npm run dev"
echo
echo "如需卸载所有服务，请运行:"
echo "make delete-test-env"
echo "make delete-ingress-controller" 
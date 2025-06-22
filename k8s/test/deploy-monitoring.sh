#!/bin/bash

# CloudDrive监控功能部署脚本
# 使用方法: ./deploy-monitoring.sh [namespace]

set -e

NAMESPACE=${1:-default}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🚀 开始部署CloudDrive监控功能到命名空间: $NAMESPACE"

# 创建命名空间（如果不存在）
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# 1. 部署监控配置
echo "📋 部署监控配置..."
kubectl apply -f $SCRIPT_DIR/monitoring-config.yaml -n $NAMESPACE

# 2. 部署增强版API服务器
echo "🔧 部署增强版API服务器..."
kubectl apply -f $SCRIPT_DIR/api-server-test-enhanced.yaml -n $NAMESPACE

# 3. 部署增强版Ingress
echo "🌐 部署增强版Ingress..."
kubectl apply -f $SCRIPT_DIR/ingress-test-enhanced.yaml -n $NAMESPACE

# 4. 等待Pod就绪
echo "⏳ 等待Pod就绪..."
kubectl wait --for=condition=ready pod -l app=api-server -n $NAMESPACE --timeout=300s

# 5. 验证健康检查
echo "🔍 验证健康检查功能..."
API_SERVER_POD=$(kubectl get pods -l app=api-server -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}')

if [ -n "$API_SERVER_POD" ]; then
    echo "测试Pod内健康检查..."
    kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s -H "X-Request-ID: deploy-test" http://localhost:8080/health | jq .
    
    echo "测试Service健康检查..."
    kubectl run test-pod --image=curlimages/curl --rm -i --restart=Never -n $NAMESPACE -- \
        curl -s -H "X-Request-ID: service-test" http://api-server:8080/health | jq .
else
    echo "❌ 未找到API服务器Pod"
    exit 1
fi

# 6. 显示部署状态
echo "📊 部署状态概览:"
echo "===================="
kubectl get pods,svc,ingress -l app=api-server -n $NAMESPACE
echo ""

# 7. 显示监控端点
echo "🔗 监控端点:"
echo "===================="
echo "健康检查: http://clouddrive.test/health"
echo "API健康检查: http://clouddrive.test/api/health"
echo "监控指标: http://monitoring.clouddrive.test/metrics"
echo "内部健康检查: http://monitoring.clouddrive.test/health"
echo ""

# 8. 显示日志查看命令
echo "📝 日志查看命令:"
echo "===================="
echo "查看应用日志: kubectl logs -l app=api-server -n $NAMESPACE -f"
echo "查看结构化日志: kubectl exec -it $API_SERVER_POD -n $NAMESPACE -- tail -f /app/logs/app.log"
echo ""

# 9. 测试请求ID追踪
echo "🔍 测试请求ID追踪功能..."
echo "发送带有自定义请求ID的请求..."
if command -v curl &> /dev/null; then
    RESPONSE=$(curl -s -H "X-Request-ID: test-trace-123" -H "Host: clouddrive.test" http://localhost/health 2>/dev/null || echo "无法连接到服务")
    echo "响应: $RESPONSE"
else
    echo "请安装curl来测试请求追踪功能"
fi

echo ""
echo "✅ CloudDrive监控功能部署完成！"
echo ""
echo "📋 下一步操作:"
echo "1. 配置DNS解析: clouddrive.test -> Ingress IP"
echo "2. 部署Prometheus来收集指标"
echo "3. 部署Grafana来可视化监控数据"
echo "4. 配置告警通知"
echo ""
echo "🔧 故障排查命令:"
echo "kubectl describe pods -l app=api-server -n $NAMESPACE"
echo "kubectl logs -l app=api-server -n $NAMESPACE --previous" 
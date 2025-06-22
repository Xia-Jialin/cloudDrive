#!/bin/bash

# CloudDrive监控功能验证脚本
# 使用方法: ./verify-monitoring.sh [namespace]

set -e

NAMESPACE=${1:-default}
FAILED_TESTS=0

echo "🔍 开始验证CloudDrive监控功能 (命名空间: $NAMESPACE)"
echo "=========================================="

# 辅助函数
run_test() {
    local test_name="$1"
    local test_command="$2"
    local expected_pattern="$3"
    
    echo -n "测试: $test_name ... "
    
    if result=$(eval "$test_command" 2>&1); then
        if [[ -z "$expected_pattern" ]] || echo "$result" | grep -q "$expected_pattern"; then
            echo "✅ 通过"
            return 0
        else
            echo "❌ 失败 (未找到期望的模式: $expected_pattern)"
            echo "实际输出: $result"
            ((FAILED_TESTS++))
            return 1
        fi
    else
        echo "❌ 失败 (命令执行错误)"
        echo "错误输出: $result"
        ((FAILED_TESTS++))
        return 1
    fi
}

# 1. 检查Pod状态
echo "1. 检查Pod状态"
echo "----------------"
run_test "API服务器Pod运行状态" \
    "kubectl get pods -l app=api-server -n $NAMESPACE --no-headers | grep Running" \
    "Running"

# 2. 检查Service状态
echo ""
echo "2. 检查Service状态"
echo "----------------"
run_test "API服务器Service存在" \
    "kubectl get svc api-server -n $NAMESPACE --no-headers" \
    "api-server"

# 3. 获取Pod名称用于后续测试
API_SERVER_POD=$(kubectl get pods -l app=api-server -n $NAMESPACE -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$API_SERVER_POD" ]; then
    echo "❌ 无法找到API服务器Pod，跳过后续测试"
    exit 1
fi

echo "使用Pod: $API_SERVER_POD"

# 4. 测试健康检查端点
echo ""
echo "3. 测试健康检查功能"
echo "----------------"
run_test "基础健康检查" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health" \
    "healthy"

run_test "带请求ID的健康检查" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s -H 'X-Request-ID: test-123' http://localhost:8080/health" \
    "test-123"

run_test "API健康检查" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/api/health" \
    "healthy"

# 5. 测试请求ID功能
echo ""
echo "4. 测试请求ID追踪"
echo "----------------"
run_test "自动生成请求ID" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s -I http://localhost:8080/health | grep X-Request-ID" \
    "X-Request-ID"

run_test "自定义请求ID返回" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s -I -H 'X-Request-ID: custom-test-456' http://localhost:8080/health | grep 'X-Request-ID: custom-test-456'" \
    "custom-test-456"

# 6. 测试健康检查响应结构
echo ""
echo "5. 测试健康检查响应结构"
echo "--------------------"
run_test "健康检查包含状态字段" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health | jq -r '.status'" \
    "healthy"

run_test "健康检查包含版本信息" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health | jq -r '.version'" \
    "1.0.0"

run_test "健康检查包含系统信息" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health | jq -r '.system_info.cpu_count'" \
    "[0-9]+"

run_test "健康检查包含内存信息" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health | jq -r '.system_info.memory_usage.alloc_mb'" \
    "[0-9.]+"

# 7. 测试日志功能
echo ""
echo "6. 测试日志功能"
echo "-------------"
run_test "应用日志目录存在" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- ls /app/logs/" \
    "app.log"

run_test "日志文件可读" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- head -1 /app/logs/app.log" \
    "timestamp"

# 8. 测试错误处理
echo ""
echo "7. 测试错误处理"
echo "-------------"
run_test "404错误格式正确" \
    "kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/nonexistent" \
    "code.*404"

# 9. 测试Service间通信
echo ""
echo "8. 测试Service间通信"
echo "-----------------"
run_test "通过Service访问健康检查" \
    "kubectl run test-pod --image=curlimages/curl --rm -i --restart=Never -n $NAMESPACE -- curl -s http://api-server:8080/health" \
    "healthy"

# 10. 检查Prometheus注解
echo ""
echo "9. 检查监控配置"
echo "-------------"
run_test "Pod包含Prometheus注解" \
    "kubectl get pods $API_SERVER_POD -n $NAMESPACE -o yaml | grep 'prometheus.io/scrape'" \
    "prometheus.io/scrape"

run_test "Service包含监控注解" \
    "kubectl get svc api-server -n $NAMESPACE -o yaml | grep 'prometheus.io/scrape'" \
    "prometheus.io/scrape"

# 11. 检查资源配置
echo ""
echo "10. 检查资源配置"
echo "-------------"
run_test "Pod资源限制配置正确" \
    "kubectl get pods $API_SERVER_POD -n $NAMESPACE -o yaml | grep -A5 'resources:'" \
    "limits"

run_test "Pod健康检查配置正确" \
    "kubectl get pods $API_SERVER_POD -n $NAMESPACE -o yaml | grep -A5 'livenessProbe:'" \
    "httpGet"

# 12. 测试性能
echo ""
echo "11. 测试性能"
echo "----------"
run_test "健康检查响应时间 < 1秒" \
    "time kubectl exec $API_SERVER_POD -n $NAMESPACE -- curl -s http://localhost:8080/health >/dev/null" \
    "real.*0m0"

# 输出总结
echo ""
echo "=========================================="
echo "📊 测试结果总结"
echo "=========================================="

if [ $FAILED_TESTS -eq 0 ]; then
    echo "✅ 所有测试通过！监控功能工作正常。"
    echo ""
    echo "🔗 可用的监控端点:"
    echo "  • 健康检查: http://clouddrive.test/health"
    echo "  • API健康检查: http://clouddrive.test/api/health"
    echo "  • 内部监控: http://monitoring.clouddrive.test/health"
    echo ""
    echo "📝 日志查看命令:"
    echo "  kubectl logs -l app=api-server -n $NAMESPACE -f"
    echo "  kubectl exec -it $API_SERVER_POD -n $NAMESPACE -- tail -f /app/logs/app.log"
    
    exit 0
else
    echo "❌ $FAILED_TESTS 个测试失败。请检查配置和部署状态。"
    echo ""
    echo "🔧 故障排查命令:"
    echo "  kubectl describe pods -l app=api-server -n $NAMESPACE"
    echo "  kubectl logs -l app=api-server -n $NAMESPACE"
    echo "  kubectl get events -n $NAMESPACE --sort-by='.lastTimestamp'"
    
    exit 1
fi 
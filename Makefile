IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive
CHUNKSERVER_IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver
WEB_IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_web
TAG ?= latest

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

.PHONY: docker-push
docker-push:
	docker push $(IMAGE_NAME):$(TAG) 

.PHONY: chunkserver-docker-build
chunkserver-docker-build:
	docker build -t $(CHUNKSERVER_IMAGE_NAME):$(TAG) -f cmd/chunkserver/Dockerfile .

.PHONY: chunkserver-docker-push
chunkserver-docker-push:
	docker push $(CHUNKSERVER_IMAGE_NAME):$(TAG)

.PHONY: web-docker-build
web-docker-build:
	docker build -t $(WEB_IMAGE_NAME):$(TAG) -f web/Dockerfile web/

.PHONY: web-docker-push
web-docker-push:
	docker push $(WEB_IMAGE_NAME):$(TAG)

.PHONY: build-all-images
build-all-images: docker-build chunkserver-docker-build web-docker-build

.PHONY: push-all-images
push-all-images: docker-push chunkserver-docker-push web-docker-push

# 测试环境部署相关命令
.PHONY: deploy-test-env
deploy-test-env: deploy-mysql deploy-redis deploy-etcd deploy-minio

.PHONY: deploy-mysql
deploy-mysql:
	kubectl apply -f k8s/test/mysql-test.yaml

.PHONY: deploy-redis
deploy-redis:
	kubectl apply -f k8s/test/redis-test.yaml

.PHONY: deploy-etcd
deploy-etcd:
	kubectl apply -f k8s/test/etcd-test.yaml

.PHONY: deploy-minio
deploy-minio:
	kubectl apply -f k8s/test/minio-test.yaml

.PHONY: deploy-chunkserver
deploy-chunkserver:
	kubectl apply -f k8s/test/chunkserver-test.yaml

.PHONY: deploy-api-server
deploy-api-server:
	kubectl apply -f k8s/test/api-server-test.yaml

.PHONY: deploy-web
deploy-web:
	kubectl apply -f k8s/test/web-test.yaml

.PHONY: install-ingress-controller
install-ingress-controller:
	kubectl apply -f k8s/test/ingress-nginx-controller.yaml
	kubectl apply -f k8s/test/ingress-rbac.yaml

.PHONY: deploy-ingress
deploy-ingress:
	kubectl apply -f k8s/test/ingress-test.yaml
	kubectl apply -f k8s/test/chunkserver-ingress.yaml
	kubectl apply -f k8s/test/grpc-ingress-test.yaml

.PHONY: deploy-all
deploy-all: deploy-test-env deploy-chunkserver deploy-api-server deploy-web deploy-ingress

.PHONY: deploy-with-ingress
deploy-with-ingress:
	./scripts/deploy_with_ingress.sh

.PHONY: scale-chunkserver
scale-chunkserver:
	kubectl scale deployment chunkserver --replicas=$(REPLICAS)

.PHONY: delete-test-env
delete-test-env:
	kubectl delete -f k8s/test/mysql-test.yaml || true
	kubectl delete -f k8s/test/redis-test.yaml || true
	kubectl delete -f k8s/test/etcd-test.yaml || true
	kubectl delete -f k8s/test/minio-test.yaml || true
	kubectl delete -f k8s/test/chunkserver-test.yaml || true 
	kubectl delete -f k8s/test/api-server-test.yaml || true
	kubectl delete -f k8s/test/web-test.yaml || true
	kubectl delete -f k8s/test/ingress-test.yaml || true
	kubectl delete -f k8s/test/chunkserver-ingress.yaml || true
	kubectl delete -f k8s/test/grpc-ingress-test.yaml || true

.PHONY: delete-ingress-controller
delete-ingress-controller:
	kubectl delete -f k8s/test/ingress-nginx-controller.yaml || true
	kubectl delete -f k8s/test/ingress-rbac.yaml || true

# 监控功能清理命令
.PHONY: delete-monitoring
delete-monitoring:
	kubectl delete -f k8s/test/api-server-test-enhanced.yaml || true
	kubectl delete -f k8s/test/ingress-test-enhanced.yaml || true
	kubectl delete -f k8s/test/monitoring-config.yaml || true
	@echo "🧹 监控功能清理完成"

.PHONY: delete-all-enhanced
delete-all-enhanced: delete-monitoring delete-test-env
	@echo "🧹 增强版CloudDrive清理完成"

# 监控功能部署相关命令
.PHONY: deploy-monitoring-config
deploy-monitoring-config:
	kubectl apply -f k8s/test/monitoring-config.yaml

.PHONY: deploy-api-server-enhanced
deploy-api-server-enhanced:
	kubectl apply -f k8s/test/api-server-test-enhanced.yaml

.PHONY: deploy-ingress-enhanced
deploy-ingress-enhanced:
	kubectl apply -f k8s/test/ingress-test-enhanced.yaml

.PHONY: deploy-monitoring
deploy-monitoring: deploy-monitoring-config deploy-api-server-enhanced deploy-ingress-enhanced
	@echo "🚀 部署监控功能完成"
	@echo "⏳ 等待Pod就绪..."
	kubectl wait --for=condition=ready pod -l app=api-server --timeout=300s || true
	@echo "✅ 监控功能部署完成！"
	@echo ""
	@echo "🔗 监控端点:"
	@echo "  • 健康检查: http://clouddrive.test/health"
	@echo "  • API健康检查: http://clouddrive.test/api/health"
	@echo "  • 监控指标: http://monitoring.clouddrive.test/metrics"

.PHONY: deploy-monitoring-with-script
deploy-monitoring-with-script:
	chmod +x k8s/test/deploy-monitoring.sh
	./k8s/test/deploy-monitoring.sh

.PHONY: verify-monitoring
verify-monitoring:
	chmod +x k8s/test/verify-monitoring.sh
	./k8s/test/verify-monitoring.sh

.PHONY: deploy-monitoring-full
deploy-monitoring-full: deploy-monitoring verify-monitoring
	@echo "🎉 监控功能完整部署和验证完成！"

# 一键部署增强版（包含监控功能）
.PHONY: deploy-all-enhanced
deploy-all-enhanced: deploy-test-env deploy-chunkserver deploy-monitoring deploy-web
	@echo "🎉 增强版CloudDrive部署完成（包含监控功能）！"

# 配置管理相关命令
.PHONY: upload-config-to-etcd
upload-config-to-etcd:
	chmod +x scripts/upload_config_to_etcd.sh
	./scripts/upload_config_to_etcd.sh

.PHONY: upload-config-to-etcd-k8s
upload-config-to-etcd-k8s:
	ETCD_ENDPOINT="etcd:2379" ./scripts/upload_config_to_etcd.sh

# 监控管理和调试命令
.PHONY: logs-api-server
logs-api-server:
	kubectl logs -l app=api-server -f --tail=100

.PHONY: logs-api-server-structured
logs-api-server-structured:
	@echo "📝 查看结构化日志（需要Pod运行）..."
	@POD_NAME=$$(kubectl get pods -l app=api-server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	if [ -n "$$POD_NAME" ]; then \
		echo "使用Pod: $$POD_NAME"; \
		kubectl exec -it $$POD_NAME -- tail -f /app/logs/app.log 2>/dev/null || echo "日志文件不存在或Pod未就绪"; \
	else \
		echo "❌ 未找到API服务器Pod"; \
	fi

.PHONY: health-check
health-check:
	@echo "🔍 执行健康检查..."
	@POD_NAME=$$(kubectl get pods -l app=api-server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	if [ -n "$$POD_NAME" ]; then \
		echo "使用Pod: $$POD_NAME"; \
		kubectl exec $$POD_NAME -- curl -s -H "X-Request-ID: makefile-health-check" http://localhost:8080/health | jq . || echo "健康检查失败"; \
	else \
		echo "❌ 未找到API服务器Pod"; \
	fi

.PHONY: monitoring-status
monitoring-status:
	@echo "📊 监控功能状态检查"
	@echo "===================="
	@echo "Pod状态:"
	kubectl get pods -l app=api-server -o wide || true
	@echo ""
	@echo "Service状态:"
	kubectl get svc api-server || true
	@echo ""
	@echo "Ingress状态:"
	kubectl get ingress clouddrive-ingress clouddrive-monitoring-ingress || true
	@echo ""
	@echo "ConfigMap状态:"
	kubectl get configmap clouddrive-monitoring-config clouddrive-log-config || true

.PHONY: port-forward-api
port-forward-api:
	@echo "🔗 端口转发API服务器到本地8080端口..."
	@echo "访问地址: http://localhost:8080/health"
	kubectl port-forward svc/api-server 8080:8080

.PHONY: debug-pod
debug-pod:
	@echo "🐛 进入API服务器Pod进行调试..."
	@POD_NAME=$$(kubectl get pods -l app=api-server -o jsonpath='{.items[0].metadata.name}' 2>/dev/null); \
	if [ -n "$$POD_NAME" ]; then \
		echo "进入Pod: $$POD_NAME"; \
		kubectl exec -it $$POD_NAME -- /bin/sh; \
	else \
		echo "❌ 未找到API服务器Pod"; \
	fi

# 帮助命令
.PHONY: help-monitoring
help-monitoring:
	@echo "🔧 CloudDrive监控功能Make命令帮助"
	@echo "=================================="
	@echo ""
	@echo "📦 部署命令:"
	@echo "  make deploy-monitoring          - 部署监控功能（基础）"
	@echo "  make deploy-monitoring-full     - 部署监控功能并验证"
	@echo "  make deploy-monitoring-with-script - 使用脚本部署监控功能"
	@echo "  make deploy-all-enhanced        - 一键部署增强版CloudDrive"
	@echo ""
	@echo "🔍 验证和调试命令:"
	@echo "  make verify-monitoring          - 验证监控功能"
	@echo "  make health-check              - 执行健康检查"
	@echo "  make monitoring-status         - 查看监控状态"
	@echo ""
	@echo "📝 日志查看命令:"
	@echo "  make logs-api-server           - 查看API服务器日志"
	@echo "  make logs-api-server-structured - 查看结构化日志文件"
	@echo ""
	@echo "🔗 网络和调试命令:"
	@echo "  make port-forward-api          - 端口转发API服务器"
	@echo "  make debug-pod                 - 进入Pod进行调试"
	@echo ""
	@echo "🧹 清理命令:"
	@echo "  make delete-monitoring         - 清理监控功能"
	@echo "  make delete-all-enhanced       - 清理增强版CloudDrive"
	@echo ""
	@echo "📋 监控端点:"
	@echo "  • 健康检查: http://clouddrive.test/health"
	@echo "  • API健康检查: http://clouddrive.test/api/health"
	@echo "  • 监控指标: http://monitoring.clouddrive.test/metrics"

# 通用帮助命令
.PHONY: help
help:
	@echo "🚀 CloudDrive Make命令帮助"
	@echo "========================="
	@echo ""
	@echo "📦 Docker镜像命令:"
	@echo "  make docker-build              - 构建API服务器镜像"
	@echo "  make chunkserver-docker-build  - 构建块服务器镜像"
	@echo "  make web-docker-build          - 构建前端镜像"
	@echo "  make build-all-images          - 构建所有镜像"
	@echo "  make push-all-images           - 推送所有镜像"
	@echo ""
	@echo "🔧 基础部署命令:"
	@echo "  make deploy-test-env           - 部署测试环境基础设施"
	@echo "  make deploy-all                - 部署所有服务（基础版）"
	@echo "  make deploy-all-enhanced       - 部署所有服务（增强版，包含监控）"
	@echo "  make delete-test-env           - 清理测试环境"
	@echo ""
	@echo "📊 监控功能命令:"
	@echo "  make help-monitoring           - 查看监控功能详细帮助"
	@echo "  make deploy-monitoring-full    - 一键部署并验证监控功能"
	@echo "  make monitoring-status         - 查看监控状态"
	@echo ""
	@echo "🔗 快速开始:"
	@echo "  1. make deploy-test-env        # 部署基础设施"
	@echo "  2. make deploy-all-enhanced    # 部署增强版应用"
	@echo "  3. make verify-monitoring      # 验证监控功能"
	@echo ""
	@echo "💡 获取更多帮助:"
	@echo "  make help-monitoring           - 监控功能详细帮助" 
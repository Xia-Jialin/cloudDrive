# CloudDrive 简化部署 Makefile
# 版本: v1.0

# 配置变量
IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive
CHUNKSERVER_IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver
WEB_IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_web
TAG ?= latest
NAMESPACE ?= default

# 颜色定义
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
NC=\033[0m

.PHONY: help
help:
	@echo -e "$(BLUE)CloudDrive 简化部署命令$(NC)"
	@echo "=========================="
	@echo ""
	@echo -e "$(GREEN)🚀 一键部署命令:$(NC)"
	@echo "  make deploy                    - Docker Compose 一键部署"
	@echo "  make deploy-k8s               - Kubernetes 简化部署"  
	@echo "  make deploy-k8s-full          - Kubernetes 完整部署"
	@echo ""
	@echo -e "$(GREEN)🔧 构建命令:$(NC)"
	@echo "  make build                    - 构建所有镜像"
	@echo "  make build-api                - 仅构建API镜像"
	@echo "  make build-chunk              - 仅构建ChunkServer镜像"
	@echo "  make build-web                - 仅构建Web镜像"
	@echo ""
	@echo -e "$(GREEN)🧹 清理命令:$(NC)"
	@echo "  make clean                    - 清理Docker Compose部署"
	@echo "  make clean-k8s                - 清理Kubernetes部署"
	@echo "  make clean-all                - 清理所有部署"
	@echo ""
	@echo -e "$(GREEN)📊 状态命令:$(NC)"
	@echo "  make status                   - 查看服务状态"
	@echo "  make logs                     - 查看服务日志"
	@echo ""
	@echo -e "$(YELLOW)💡 快速开始:$(NC)"
	@echo "  1. make deploy                # 本地开发"
	@echo "  2. make deploy-k8s            # 生产环境"

# =============================================================================
# 构建命令
# =============================================================================

.PHONY: build
build: build-api build-chunk build-web
	@echo -e "$(GREEN)✅ 所有镜像构建完成$(NC)"

.PHONY: build-api
build-api:
	@echo -e "$(GREEN)🔨 构建API服务器镜像...$(NC)"
	@docker build -t $(IMAGE_NAME):$(TAG) .

.PHONY: build-chunk
build-chunk:
	@echo -e "$(GREEN)🔨 构建ChunkServer镜像...$(NC)"
	@docker build -t $(CHUNKSERVER_IMAGE_NAME):$(TAG) -f cmd/chunkserver/Dockerfile .

.PHONY: build-web
build-web:
	@echo -e "$(GREEN)🔨 构建Web前端镜像...$(NC)"
	@docker build -t $(WEB_IMAGE_NAME):$(TAG) -f web/Dockerfile web/

# =============================================================================
# 部署命令
# =============================================================================

.PHONY: deploy
deploy: build
	@echo -e "$(GREEN)🚀 开始Docker Compose部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -t docker-compose

.PHONY: deploy-k8s
deploy-k8s: build
	@echo -e "$(GREEN)🚀 开始Kubernetes简化部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -t k8s-simple -n $(NAMESPACE)

.PHONY: deploy-k8s-full
deploy-k8s-full: build
	@echo -e "$(GREEN)🚀 开始Kubernetes完整部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -t k8s -n $(NAMESPACE) -m

.PHONY: deploy-no-build
deploy-no-build:
	@echo -e "$(GREEN)🚀 跳过构建，直接部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -t docker-compose -s

.PHONY: deploy-k8s-no-build
deploy-k8s-no-build:
	@echo -e "$(GREEN)🚀 跳过构建，Kubernetes部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -t k8s-simple -n $(NAMESPACE) -s

# =============================================================================
# 清理命令
# =============================================================================

.PHONY: clean
clean:
	@echo -e "$(YELLOW)🧹 清理Docker Compose部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -c -t docker-compose

.PHONY: clean-k8s
clean-k8s:
	@echo -e "$(YELLOW)🧹 清理Kubernetes部署...$(NC)"
	@chmod +x deploy-simple.sh
	@./deploy-simple.sh -c -t k8s-simple -n $(NAMESPACE)

.PHONY: clean-all
clean-all: clean clean-k8s
	@echo -e "$(YELLOW)🧹 清理Docker镜像...$(NC)"
	@docker rmi $(IMAGE_NAME):$(TAG) $(CHUNKSERVER_IMAGE_NAME):$(TAG) $(WEB_IMAGE_NAME):$(TAG) 2>/dev/null || true
	@docker system prune -f
	@echo -e "$(GREEN)✅ 清理完成$(NC)"

# =============================================================================
# 状态和日志命令
# =============================================================================

.PHONY: status
status:
	@echo -e "$(BLUE)📊 服务状态检查$(NC)"
	@echo "===================="
	@echo ""
	@echo -e "$(GREEN)Docker Compose 服务:$(NC)"
	@docker-compose ps 2>/dev/null || echo "Docker Compose 未运行"
	@echo ""
	@echo -e "$(GREEN)Kubernetes 服务:$(NC)"
	@kubectl get pods,svc -n $(NAMESPACE) 2>/dev/null || echo "Kubernetes 未部署或无权限"

.PHONY: logs
logs:
	@echo -e "$(BLUE)📝 查看服务日志$(NC)"
	@echo "===================="
	@echo ""
	@echo -e "$(GREEN)Docker Compose 日志:$(NC)"
	@docker-compose logs --tail=50 2>/dev/null || echo "Docker Compose 未运行"
	@echo ""
	@echo -e "$(GREEN)Kubernetes 日志:$(NC)"
	@kubectl logs -l app=api-server -n $(NAMESPACE) --tail=50 2>/dev/null || echo "Kubernetes 未部署"

.PHONY: logs-follow
logs-follow:
	@echo -e "$(BLUE)📝 实时查看API服务器日志$(NC)"
	@docker-compose logs -f api-server 2>/dev/null || kubectl logs -l app=api-server -n $(NAMESPACE) -f 2>/dev/null || echo "服务未运行"

# =============================================================================
# 开发命令
# =============================================================================

.PHONY: dev
dev:
	@echo -e "$(GREEN)🛠️  启动开发环境...$(NC)"
	@docker-compose -f docker-compose.yml up mysql redis etcd -d
	@echo -e "$(GREEN)✅ 开发环境基础服务已启动$(NC)"
	@echo "现在可以本地运行 go run cmd/server/main.go"

.PHONY: dev-stop
dev-stop:
	@echo -e "$(YELLOW)🛑 停止开发环境...$(NC)"
	@docker-compose -f docker-compose.yml stop
	@echo -e "$(GREEN)✅ 开发环境已停止$(NC)"

# =============================================================================
# 工具命令
# =============================================================================

.PHONY: health
health:
	@echo -e "$(BLUE)🔍 健康检查$(NC)"
	@echo "===================="
	@curl -s http://localhost:8080/api/health 2>/dev/null | jq . || echo "API服务器未响应"

.PHONY: test-endpoints
test-endpoints:
	@echo -e "$(BLUE)🧪 测试API端点$(NC)"
	@echo "===================="
	@echo "健康检查:"
	@curl -s http://localhost:8080/api/health || echo "失败"
	@echo ""
	@echo "用户注册测试:"
	@curl -s -X POST http://localhost:8080/api/user/register \
		-H "Content-Type: application/json" \
		-d '{"email":"test@example.com","password":"123456"}' || echo "失败"

# =============================================================================
# 镜像管理命令
# =============================================================================

.PHONY: push
push: build
	@echo -e "$(GREEN)📤 推送镜像到仓库...$(NC)"
	@docker push $(IMAGE_NAME):$(TAG)
	@docker push $(CHUNKSERVER_IMAGE_NAME):$(TAG)
	@docker push $(WEB_IMAGE_NAME):$(TAG)
	@echo -e "$(GREEN)✅ 镜像推送完成$(NC)"

.PHONY: pull
pull:
	@echo -e "$(GREEN)📥 拉取镜像...$(NC)"
	@docker pull $(IMAGE_NAME):$(TAG)
	@docker pull $(CHUNKSERVER_IMAGE_NAME):$(TAG)
	@docker pull $(WEB_IMAGE_NAME):$(TAG)
	@echo -e "$(GREEN)✅ 镜像拉取完成$(NC)"

# =============================================================================
# 兼容性命令（保持与原Makefile的兼容性）
# =============================================================================

.PHONY: docker-build
docker-build: build-api

.PHONY: build-all-images
build-all-images: build

.PHONY: deploy-all
deploy-all: deploy-k8s

.PHONY: delete-test-env
delete-test-env: clean-k8s 
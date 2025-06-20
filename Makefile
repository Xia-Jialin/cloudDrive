IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive
CHUNKSERVER_IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive_chunkserver
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

.PHONY: build-all-images
build-all-images: docker-build chunkserver-docker-build

.PHONY: push-all-images
push-all-images: docker-push chunkserver-docker-push

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
deploy-all: deploy-test-env deploy-chunkserver deploy-api-server deploy-ingress

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
	kubectl delete -f k8s/test/ingress-test.yaml || true
	kubectl delete -f k8s/test/chunkserver-ingress.yaml || true
	kubectl delete -f k8s/test/grpc-ingress-test.yaml || true

.PHONY: delete-ingress-controller
delete-ingress-controller:
	kubectl delete -f k8s/test/ingress-nginx-controller.yaml || true
	kubectl delete -f k8s/test/ingress-rbac.yaml || true

# 配置管理相关命令
.PHONY: upload-config-to-etcd
upload-config-to-etcd:
	chmod +x scripts/upload_config_to_etcd.sh
	./scripts/upload_config_to_etcd.sh

.PHONY: upload-config-to-etcd-k8s
upload-config-to-etcd-k8s:
	ETCD_ENDPOINT="etcd:2379" ./scripts/upload_config_to_etcd.sh 
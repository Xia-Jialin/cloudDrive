IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive
TAG ?= latest

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

.PHONY: docker-push
docker-push:
	docker push $(IMAGE_NAME):$(TAG) 

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

.PHONY: delete-test-env
delete-test-env:
	kubectl delete -f k8s/test/mysql-test.yaml || true
	kubectl delete -f k8s/test/redis-test.yaml || true
	kubectl delete -f k8s/test/etcd-test.yaml || true
	kubectl delete -f k8s/test/minio-test.yaml || true 
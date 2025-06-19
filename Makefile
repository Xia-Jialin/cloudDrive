IMAGE_NAME=registry.cn-hangzhou.aliyuncs.com/xiajialin/cloud_drive
TAG ?= latest

.PHONY: docker-build
docker-build:
	docker build -t $(IMAGE_NAME):$(TAG) .

.PHONY: docker-push
docker-push:
	docker push $(IMAGE_NAME):$(TAG) 
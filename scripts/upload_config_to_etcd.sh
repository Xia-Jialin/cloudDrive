#!/bin/bash

# 设置默认值
ETCD_ENDPOINT=${ETCD_ENDPOINT:-"localhost:2379"}
SERVER_CONFIG_KEY=${SERVER_CONFIG_KEY:-"/clouddrive/server/config"}
CHUNKSERVER_CONFIG_KEY=${CHUNKSERVER_CONFIG_KEY:-"/clouddrive/chunkserver/config"}

echo "编译配置上传工具..."
go build -o config_uploader scripts/config_to_etcd.go

echo "上传主服务配置到ETCD..."
./config_uploader --config configs/config.yaml --etcd "${ETCD_ENDPOINT}" --key "${SERVER_CONFIG_KEY}"

echo "上传块存储服务配置到ETCD..."
./config_uploader --config configs/chunkserver.yaml --etcd "${ETCD_ENDPOINT}" --key "${CHUNKSERVER_CONFIG_KEY}"

echo "配置上传完成！"

# 清理
rm config_uploader 
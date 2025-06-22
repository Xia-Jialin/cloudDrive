# Ingress 部署方案

本文档描述了如何使用 Kubernetes Ingress 暴露 CloudDrive 服务。

## 前提条件

1. 已安装 Kubernetes 集群
2. 已安装 Nginx Ingress Controller（如果未安装，请参考下面的安装说明）
3. 已部署 CloudDrive 基础服务（MySQL、Redis、etcd、MinIO）

## 安装 Nginx Ingress Controller

如果您的集群尚未安装 Nginx Ingress Controller，可以使用以下命令安装：

```bash
# 使用项目提供的 Makefile 命令安装
make install-ingress-controller

# 或者使用 Helm 安装
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
helm install ingress-nginx ingress-nginx/ingress-nginx

# 或者直接使用 kubectl 安装
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/cloud/deploy.yaml
```

安装后，等待 Ingress Controller 就绪：

```bash
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s
```

## 部署 CloudDrive 服务

您可以使用自动化脚本一键部署所有服务（包括安装 Ingress Controller）：

```bash
make deploy-with-ingress
```

或者按照以下步骤手动部署：

1. 部署基础服务：

```bash
make deploy-test-env
```

2. 将配置上传到 etcd：

```bash
make upload-config-to-etcd-k8s
```

3. 部署 API 服务器和块存储服务器：

```bash
make deploy-api-server
make deploy-chunkserver
```

4. 部署 Ingress 规则：

```bash
make deploy-ingress
```

## 访问服务

### 获取 Ingress 控制器的 IP 地址和端口

```bash
kubectl get service -n ingress-nginx
```

您将看到类似以下输出：

```
NAME                       TYPE       CLUSTER-IP        EXTERNAL-IP   PORT(S)                      AGE
ingress-nginx-controller   NodePort   192.168.194.191   <none>        80:30080/TCP,443:30443/TCP   3m17s
```

记下 NodePort 端口号（在本例中为 30080）和节点 IP 地址：

```bash
kubectl get nodes -o wide
```

### 配置本地 hosts 文件

为了通过域名访问服务，需要在本地 hosts 文件中添加以下条目：

```
<节点IP地址> clouddrive.local chunk.clouddrive.local grpc.clouddrive.local
```

例如：

```
198.19.249.2 clouddrive.local chunk.clouddrive.local grpc.clouddrive.local
```

### 访问 API 服务

通过以下方式访问 API 服务：

1. 使用域名（需要配置 hosts 文件）：

```bash
curl -v "http://clouddrive.local:30080/api/health"
```

2. 直接使用 IP 地址和 Host 头：

```bash
curl -v "http://<节点IP地址>:30080/api/health" -H "Host: clouddrive.local"
```

### 访问块存储服务

通过以下方式访问块存储服务：

1. 使用域名（需要配置 hosts 文件）：

```bash
curl -v "http://chunk.clouddrive.local:30080/api/health"
```

2. 直接使用 IP 地址和 Host 头：

```bash
curl -v "http://<节点IP地址>:30080/api/health" -H "Host: chunk.clouddrive.local"
```

### 前端配置

修改前端环境变量，使其指向 Ingress 地址：

```bash
# 开发环境（使用域名）
VITE_API_BASE_URL=http://clouddrive.local:30080 npm run dev

# 开发环境（使用IP地址）
VITE_API_BASE_URL=http://<节点IP地址>:30080 VITE_API_HOST=clouddrive.local npm run dev

# 或者在 .env 文件中设置
echo "VITE_API_BASE_URL=http://<节点IP地址>:30080" > web/.env
echo "VITE_API_HOST=clouddrive.local" >> web/.env
```

## 注意事项

1. **Host 头设置**：使用 Ingress 时，必须设置正确的 Host 头。前端应用的 Vite 配置已更新，自动设置正确的 Host 头。

2. **gRPC 支持**：我们使用了专门的 Ingress 配置来支持 gRPC 服务，通过 `grpc.clouddrive.local` 域名访问。这需要 Nginx Ingress Controller 支持 gRPC 协议。

3. **文件大小限制**：默认配置中已设置允许上传的最大文件大小为 100MB。如需调整，请修改 Ingress 配置中的 `nginx.ingress.kubernetes.io/proxy-body-size` 注解。

4. **HTTPS**：生产环境建议配置 HTTPS。可以使用 cert-manager 自动管理证书，或手动配置 TLS 证书。

## 故障排除

1. 检查 Ingress 状态：

```bash
kubectl get ingress
kubectl describe ingress clouddrive-ingress
kubectl describe ingress chunkserver-ingress
kubectl describe ingress clouddrive-grpc-ingress
```

2. 检查 Ingress 控制器日志：

```bash
kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx
```

3. 检查服务是否正常运行：

```bash
kubectl get pods
kubectl logs -l app=api-server
kubectl logs -l app=chunkserver
```

4. 测试服务连接：

```bash
# 使用端口转发直接访问服务
kubectl port-forward svc/api-server 8080:8080
curl -v "http://localhost:8080/api/health"
```

5. 如果使用的是 Minikube 或类似的本地 Kubernetes 环境，可能需要使用端口转发来访问服务：

```bash
# 端口转发 Ingress 控制器
kubectl port-forward -n ingress-nginx service/ingress-nginx-controller 8080:80
```

然后在 hosts 文件中添加：

```
127.0.0.1 clouddrive.local chunk.clouddrive.local grpc.clouddrive.local
```

并通过 `http://clouddrive.local:8080/api` 访问服务。 
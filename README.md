# cloudDrive 用户注册后端

## 技术栈
- Go 1.20+
- Gin
- GORM
- MySQL

## 目录结构
```
cloudDrive/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   └── user/
│       ├── model.go
│       ├── register.go
│       └── util.go
├── go.mod
```

## 数据库准备
```sql
CREATE DATABASE clouddrive DEFAULT CHARACTER SET utf8mb4;
```

## 启动服务
```bash
cd cmd/server
# 确保 go.mod 在项目根目录
# 安装依赖
cd ../..
go mod tidy
go run cmd/server/main.go
```

## 注册接口
- URL: `POST /api/user/register`
- 请求体：
```
{
  "email": "test@example.com", // 或 phone
  "password": "yourpassword"
}
```
- 返回：
```
{
  "id": 1
}
```

# Docker 部署说明

## 一键启动（推荐）

1. 构建并启动所有服务（后端、MySQL、Redis）：

```bash
docker-compose up --build -d
```

2. 访问后端 API（默认端口 8080）：

```
http://localhost:8080/swagger/index.html
```

3. 文件存储目录映射在本地 `cmd/server/uploads`，可持久化。

## 仅构建后端镜像

```bash
docker build -t clouddrive-server .
docker run -d -p 8080:8080 -v $(pwd)/cmd/server/uploads:/app/uploads clouddrive-server
```

## 配置说明
- 配置文件位于 `configs/config.yaml`，可通过环境变量 `CONFIG_PATH` 指定。
- 如需通过环境变量覆盖数据库、Redis 等参数，建议扩展 viper 读取逻辑（支持 `viper.BindEnv`）。

## 注意事项
- 首次启动请确保 3306、6379、8080 端口未被占用。
- 如需自定义存储目录或数据库密码，请同步修改 `docker-compose.yml` 和 `configs/config.yaml`。

# 前端部署说明

## 本地开发

```bash
cd web
npm install
npm run dev
```

访问：http://localhost:5173

## 生产构建

```bash
cd web
npm install
npm run build
# dist/ 目录为静态资源，可用 nginx、http-server 等托管
```

## Docker 部署（推荐）

1. 一键启动前后端、数据库、Redis：

```bash
docker-compose up --build -d
```

2. 访问前端页面：http://localhost/

3. 前端容器自动代理 /api 请求到后端 clouddrive 服务。 
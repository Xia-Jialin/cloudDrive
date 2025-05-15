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
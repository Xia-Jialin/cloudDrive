# CloudDrive Web 前端

## 启动开发环境

1. 安装依赖

```bash
cd web
npm install
```

2. 启动开发服务器

```bash
npm run dev
```

3. 访问页面

浏览器打开 http://localhost:5173

## 功能
- 注册、登录、退出登录
- 登录信息本地持久化
- 登录后显示用户信息

## 注意
- 需后端服务（Gin）已启动并监听 8080 端口
- 前端已配置接口代理，无需修改API地址 
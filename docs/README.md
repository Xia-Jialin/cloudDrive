# 项目文档

本文档库包含项目的所有相关文档，包括 API、设计和部署文档。

## 目录结构

-   [**`api/`**](./api/): 包含所有 API 定义和规范文档。
    -   `swagger.json` / `swagger.yaml`: OpenAPI 规范文件。
-   [**`design/`**](./design/): 包含产品需求、功能设计和数据库设计文档。
    -   [**`recycle_restore/`**](./design/recycle_restore/): 回收站和恢复功能的设计文档。
    -   [**`需求文档.md`**](./design/需求文档.md): 项目总体需求文档。
    -   [**`用户模块详细说明.md`**](./design/用户模块详细说明.md): 用户模块的设计文档。
    -   [**`文件管理模块.md`**](./design/文件管理模块.md): 文件管理模块的设计文档。
-   [**`deployment/`**](./deployment/): 包含与项目部署相关的文档。
    -   [**`ingress_deployment.md`**](./deployment/ingress_deployment.md): Ingress 部署指南。
-   [**`docs.go`**](./docs.go): `swaggo` 自动生成的 Go 代码，用于生成 Swagger 文档。

---

请保持此文档结构的整洁，新的文档请根据分类放置在对应的目录下。 
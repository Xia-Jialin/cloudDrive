package main

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/handler"
	"cloudDrive/internal/user"
	"log"

	_ "cloudDrive/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// @title CloudDrive API
// @version 1.0
// @description 云盘系统后端 API 文档
// @host localhost:8080
// @BasePath /api

var db *gorm.DB

func main() {
	dsn := "root:123456@tcp(127.0.0.1:3306)/clouddrive?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表和文件表，并捕获错误
	err = db.AutoMigrate(&user.User{}, &file.File{}, &file.FileContent{}, &file.UserRoot{}, &file.Share{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	r := gin.Default()

	// 注入 db 到 gin.Context
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})

	r.POST("/api/user/register", handler.RegisterHandler)
	r.POST("/api/user/login", handler.LoginHandler)

	r.GET("/api/files", handler.FileListHandler)
	r.POST("/api/files/upload", handler.FileUploadHandler)
	r.GET("/api/files/download/:id", handler.FileDownloadHandler)
	r.DELETE("/api/files/:id", handler.FileDeleteHandler)
	r.PUT("/api/files/:id/rename", handler.FileRenameHandler)
	r.POST("/api/folders", handler.CreateFolderHandler)
	r.PUT("/api/files/:id/move", handler.FileMoveHandler)
	r.POST("/api/share/public", handler.CreatePublicShareHandler)
	r.GET("/api/share/public", handler.GetPublicShareHandler)
	r.GET("/api/share/:token", handler.AccessShareHandler)
	r.GET("/api/share/download/:token", handler.ShareDownloadHandler)
	r.POST("/api/share/private", handler.CreatePrivateShareHandler)
	r.GET("/api/share/private", handler.GetPrivateShareHandler)
	r.DELETE("/api/share", handler.CancelShareHandler)
	r.GET("/api/files/search", handler.FileSearchHandler)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Run(":8080")
}

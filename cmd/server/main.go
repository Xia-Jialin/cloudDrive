package main

import (
	"cloudDrive/internal/user"
	"log"
	"net/http"

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

// @Summary 用户注册
// @Description 注册新用户
// @Tags 用户模块
// @Accept json
// @Produce json
// @Param data body user.RegisterRequest true "注册参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/register [post]
func RegisterHandler(c *gin.Context) {
	var req user.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "detail": err.Error()})
		return
	}
	resp, err := user.Register(db, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": resp.ID})
}

// @Summary 用户登录
// @Description 用户登录
// @Tags 用户模块
// @Accept json
// @Produce json
// @Param data body user.LoginRequest true "登录参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /user/login [post]
func LoginHandler(c *gin.Context) {
	var req user.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "detail": err.Error()})
		return
	}
	resp, err := user.Login(db, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func main() {
	dsn := "root:123456@tcp(127.0.0.1:3306)/clouddrive?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表
	db.AutoMigrate(&user.User{})

	r := gin.Default()

	r.POST("/api/user/register", RegisterHandler)
	r.POST("/api/user/login", LoginHandler)

	// 在r.Run前注册Swagger路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Run(":8080")
}

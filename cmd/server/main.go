package main

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/user"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "cloudDrive/docs"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

// @Summary 获取文件/文件夹列表
// @Description 获取指定目录下的文件和文件夹，支持分页和排序
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param parent_id query int false "父目录ID，根目录为0"
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Param order_by query string false "排序字段，默认upload_time"
// @Param order query string false "排序方式，asc/desc，默认desc"
// @Success 200 {object} file.ListFilesResponse
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files [get]
func FileListHandler(c *gin.Context) {
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
		tokenStr = tokenStr[7:]
	}
	claims := user.Claims{}
	secret := "cloudDriveSecret"
	parsed, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或Token无效"})
		return
	}
	parentID, _ := strconv.ParseUint(c.DefaultQuery("parent_id", "0"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	orderBy := c.DefaultQuery("order_by", "upload_time")
	order := c.DefaultQuery("order", "desc")
	resp, err := file.ListFiles(db, file.ListFilesRequest{
		ParentID: uint(parentID),
		OwnerID:  claims.UserID,
		Page:     page,
		PageSize: pageSize,
		OrderBy:  orderBy,
		Order:    order,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// @Summary 上传文件
// @Description 上传文件到指定目录
// @Tags 文件模块
// @Accept multipart/form-data
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param file formData file true "文件"
// @Param parent_id formData int false "父目录ID，根目录为0"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/upload [post]
func FileUploadHandler(c *gin.Context) {
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
		tokenStr = tokenStr[7:]
	}
	claims := user.Claims{}
	secret := "cloudDriveSecret"
	parsed, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !parsed.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录或Token无效"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未选择文件"})
		return
	}
	parentIDStr := c.PostForm("parent_id")
	parentID := uint(0)
	if parentIDStr != "" {
		if v, err := strconv.ParseUint(parentIDStr, 10, 64); err == nil {
			parentID = uint(v)
		}
	}
	// 保存文件到本地（可根据实际需求调整存储路径）
	savePath := "uploads/" + fileHeader.Filename
	if err := c.SaveUploadedFile(fileHeader, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败", "detail": err.Error()})
		return
	}
	// 写入数据库
	f := file.File{
		Name:       fileHeader.Filename,
		Size:       fileHeader.Size,
		Type:       "file",
		ParentID:   parentID,
		OwnerID:    claims.UserID,
		UploadTime: time.Now(),
	}
	if err := db.Create(&f).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": f.ID, "name": f.Name, "size": f.Size})
}

func main() {
	dsn := "root:123456@tcp(127.0.0.1:3306)/clouddrive?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表和文件表，并捕获错误
	err = db.AutoMigrate(&user.User{}, &file.File{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	r := gin.Default()

	r.POST("/api/user/register", RegisterHandler)
	r.POST("/api/user/login", LoginHandler)

	// 文件列表获取接口
	r.GET("/api/files", FileListHandler)

	// 文件上传接口
	r.POST("/api/files/upload", FileUploadHandler)

	// 在r.Run前注册Swagger路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Run(":8080")
}

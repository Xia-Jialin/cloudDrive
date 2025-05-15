package main

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/user"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
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
// @Param parent_id query string false "父目录ID，根目录为0"
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
	parentID := c.DefaultQuery("parent_id", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	orderBy := c.DefaultQuery("order_by", "upload_time")
	order := c.DefaultQuery("order", "desc")
	resp, err := file.ListFiles(db, file.ListFilesRequest{
		ParentID: parentID,
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

	// 为每个文件补充size字段
	filesWithSize := make([]gin.H, 0, len(resp.Files))
	for _, f := range resp.Files {
		fileMap := gin.H{
			"id":          f.ID,
			"name":        f.Name,
			"hash":        f.Hash,
			"type":        f.Type,
			"parent_id":   f.ParentID,
			"owner_id":    f.OwnerID,
			"upload_time": f.UploadTime,
		}
		if f.Type == "file" {
			var fc file.FileContent
			db.First(&fc, "hash = ?", f.Hash)
			fileMap["size"] = fc.Size
		} else {
			fileMap["size"] = nil
		}
		filesWithSize = append(filesWithSize, fileMap)
	}
	c.JSON(http.StatusOK, gin.H{"files": filesWithSize, "total": resp.Total})
}

// @Summary 上传文件
// @Description 上传文件到指定目录
// @Tags 文件模块
// @Accept multipart/form-data
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param file formData file true "文件"
// @Param parent_id formData string false "父目录ID，根目录为0"
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
	parentID := c.PostForm("parent_id")

	// 1. 读取文件内容并计算哈希
	fileObj, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件读取失败", "detail": err.Error()})
		return
	}
	defer fileObj.Close()

	hashObj := sha256.New()
	fileBytes, err := io.ReadAll(io.TeeReader(fileObj, hashObj))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件读取失败", "detail": err.Error()})
		return
	}
	hashStr := hex.EncodeToString(hashObj.Sum(nil))

	// 2. 检查 FileContent 是否已存在
	var fileContent file.FileContent
	err = db.First(&fileContent, "hash = ?", hashStr).Error
	if err == gorm.ErrRecordNotFound {
		// 保存文件到本地，文件名为 hash
		savePath := "uploads/" + hashStr
		err = os.WriteFile(savePath, fileBytes, 0644)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败", "detail": err.Error()})
			return
		}
		// 插入 FileContent
		fileContent = file.FileContent{
			Hash: hashStr,
			Size: fileHeader.Size,
		}
		err = db.Create(&fileContent).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败", "detail": err.Error()})
		return
	}

	// 3. File 表插入记录
	f := file.File{
		Name:       fileHeader.Filename,
		Hash:       hashStr,
		Type:       "file",
		ParentID:   parentID,
		OwnerID:    claims.UserID,
		UploadTime: time.Now(),
	}
	if err := db.Create(&f).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": f.ID, "name": f.Name, "size": fileContent.Size})
}

// @Summary 下载文件
// @Description 下载指定文件
// @Tags 文件模块
// @Accept json
// @Produce application/octet-stream
// @Param Authorization header string true "Bearer Token"
// @Param id path string true "文件ID"
// @Success 200 {file} file
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/download/{id} [get]
func FileDownloadHandler(c *gin.Context) {
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
	idStr := c.Param("id")
	var f file.File
	err = db.First(&f, "id = ?", idStr).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}
	if f.OwnerID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限下载该文件"})
		return
	}
	if f.Type != "file" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能下载文件类型"})
		return
	}
	filePath := "uploads/" + f.Hash
	c.FileAttachment(filePath, f.Name)
}

// @Summary 删除文件
// @Description 删除指定文件
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param id path string true "文件ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/{id} [delete]
func FileDeleteHandler(c *gin.Context) {
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
	idStr := c.Param("id")
	err = file.DeleteFile(db, idStr, claims.UserID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		if err == file.ErrNoPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限删除该文件"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// @Summary 重命名文件
// @Description 重命名指定文件
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param id path string true "文件ID"
// @Param data body file.RenameFileRequest true "新文件名"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/{id}/rename [put]
func FileRenameHandler(c *gin.Context) {
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
	idStr := c.Param("id")
	var req file.RenameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.NewName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新文件名不能为空"})
		return
	}
	err = file.RenameFile(db, idStr, claims.UserID, req.NewName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		if err == file.ErrNoPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限重命名该文件"})
			return
		}
		if err == file.ErrNameExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "同目录下已存在同名文件"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "重命名成功"})
}

func main() {
	dsn := "root:123456@tcp(127.0.0.1:3306)/clouddrive?charset=utf8mb4&parseTime=True&loc=Local"
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表和文件表，并捕获错误
	err = db.AutoMigrate(&user.User{}, &file.File{}, &file.FileContent{})
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

	// 文件下载接口
	r.GET("/api/files/download/:id", FileDownloadHandler)

	// 文件删除接口
	r.DELETE("/api/files/:id", FileDeleteHandler)

	// 文件重命名接口
	r.PUT("/api/files/:id/rename", FileRenameHandler)

	// 在r.Run前注册Swagger路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Run(":8080")
}

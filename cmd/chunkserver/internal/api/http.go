package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cloudDrive/cmd/chunkserver/internal/config"
	"cloudDrive/cmd/chunkserver/internal/service"
	"cloudDrive/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v4"
)

// HTTPServer HTTP服务器
type HTTPServer struct {
	service *service.StorageServiceImpl
	redis   *redis.Client
	server  *http.Server
}

// NewHTTPServer 创建HTTP服务器
func NewHTTPServer(service *service.StorageServiceImpl, redis *redis.Client, port int) *HTTPServer {
	router := gin.Default()

	// 注册路由
	router.POST("/upload", handleDirectUpload(service))
	router.GET("/download", handleDirectDownload(service))
	router.POST("/delete", handleDirectDelete(service))

	// 分片上传API
	router.POST("/multipart/init", handleInitMultipart(service))
	router.POST("/multipart/upload", handleUploadPart(service))
	router.POST("/multipart/complete", handleCompleteMultipart(service))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	return &HTTPServer{
		service: service,
		redis:   redis,
		server:  server,
	}
}

// Start 启动HTTP服务器
func (s *HTTPServer) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown 关闭HTTP服务器
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// 处理直接上传请求
func handleDirectUpload(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.PostForm("token")
		if token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "token参数必填",
			})
			return
		}

		// 验证令牌
		tokenInfo, err := service.VerifyToken(c.Request.Context(), token, "upload")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    2,
				"message": err.Error(),
			})
			return
		}

		// 获取上传文件
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    3,
				"message": fmt.Sprintf("获取文件失败: %v", err),
			})
			return
		}

		// 打开文件
		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    4,
				"message": fmt.Sprintf("打开文件失败: %v", err),
			})
			return
		}
		defer f.Close()

		// 获取文件ID
		fileID, ok := tokenInfo["file_id"].(string)
		if !ok || fileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    5,
				"message": "无效的文件ID",
			})
			return
		}

		// 保存文件
		if err := service.Save(c.Request.Context(), fileID, f); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    6,
				"message": fmt.Sprintf("保存文件失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "上传成功",
			"data": gin.H{
				"file_id": fileID,
				"size":    file.Size,
			},
		})
	}
}

// 处理直接下载请求
func handleDirectDownload(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := c.Query("file_id")
		token := c.Query("token")

		if fileID == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "file_id和token参数必填",
			})
			return
		}

		// 验证令牌
		tokenInfo, err := service.VerifyToken(c.Request.Context(), token, "download")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    2,
				"message": err.Error(),
			})
			return
		}

		// 验证文件ID是否匹配
		storedFileID, ok := tokenInfo["file_id"].(string)
		if !ok || storedFileID != fileID {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    3,
				"message": "令牌与请求文件不匹配",
			})
			return
		}

		// 读取文件内容
		data, err := service.Read(c.Request.Context(), fileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    4,
				"message": fmt.Sprintf("读取文件失败: %v", err),
			})
			return
		}

		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileID))
		c.Data(http.StatusOK, "application/octet-stream", data)
	}
}

// 处理直接删除请求
func handleDirectDelete(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := c.PostForm("file_id")
		token := c.PostForm("token")

		if fileID == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "file_id和token参数必填",
			})
			return
		}

		// 验证令牌
		tokenInfo, err := service.VerifyToken(c.Request.Context(), token, "delete")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    2,
				"message": err.Error(),
			})
			return
		}

		// 验证文件ID是否匹配
		storedFileID, ok := tokenInfo["file_id"].(string)
		if !ok || storedFileID != fileID {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    3,
				"message": "令牌与请求文件不匹配",
			})
			return
		}

		// 删除文件
		if err := service.Delete(c.Request.Context(), fileID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    4,
				"message": fmt.Sprintf("删除文件失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "删除成功",
		})
	}
}

// 处理初始化分片上传请求
func handleInitMultipart(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		filename := c.Query("filename")
		token := c.Query("token")

		if filename == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "filename和token参数必填",
			})
			return
		}

		// 验证令牌
		tokenInfo, err := service.VerifyToken(c.Request.Context(), token, "multipart_init")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    2,
				"message": err.Error(),
			})
			return
		}

		// 获取文件ID
		fileID, ok := tokenInfo["file_id"].(string)
		if !ok || fileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    3,
				"message": "无效的文件ID",
			})
			return
		}

		// 初始化分片上传
		uploadID, err := service.InitMultipartUpload(c.Request.Context(), fileID, filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    4,
				"message": fmt.Sprintf("初始化分片上传失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "初始化成功",
			"data": gin.H{
				"upload_id": uploadID,
			},
		})
	}
}

// 处理上传分片请求
func handleUploadPart(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadID := c.PostForm("upload_id")
		partNumberStr := c.PostForm("part_number")
		token := c.PostForm("token")

		if uploadID == "" || partNumberStr == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "upload_id、part_number和token参数必填",
			})
			return
		}

		partNumber, err := strconv.Atoi(partNumberStr)
		if err != nil || partNumber <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    2,
				"message": "part_number参数无效",
			})
			return
		}

		// 验证令牌
		_, err = service.VerifyToken(c.Request.Context(), token, "multipart_upload")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    3,
				"message": err.Error(),
			})
			return
		}

		// 获取分片文件
		file, err := c.FormFile("part")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4,
				"message": fmt.Sprintf("获取分片失败: %v", err),
			})
			return
		}

		// 打开分片文件
		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5,
				"message": fmt.Sprintf("打开分片失败: %v", err),
			})
			return
		}
		defer f.Close()

		// 上传分片
		etag, err := service.UploadPart(c.Request.Context(), uploadID, partNumber, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    6,
				"message": fmt.Sprintf("上传分片失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "上传分片成功",
			"data": gin.H{
				"etag": etag,
			},
		})
	}
}

// 处理完成分片上传请求
func handleCompleteMultipart(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadID := c.PostForm("upload_id")
		partsJSON := c.PostForm("parts")
		token := c.PostForm("token")

		if uploadID == "" || partsJSON == "" || token == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "upload_id、parts和token参数必填",
			})
			return
		}

		// 验证令牌
		_, err := service.VerifyToken(c.Request.Context(), token, "multipart_complete")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    2,
				"message": err.Error(),
			})
			return
		}

		// 解析分片信息
		var parts []storage.PartInfo
		if err := json.Unmarshal([]byte(partsJSON), &parts); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    3,
				"message": fmt.Sprintf("解析分片信息失败: %v", err),
			})
			return
		}

		// 完成分片上传
		fileID, err := service.CompleteMultipartUpload(c.Request.Context(), uploadID, parts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    4,
				"message": fmt.Sprintf("完成分片上传失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "完成分片上传成功",
			"data": gin.H{
				"file_id": fileID,
			},
		})
	}
}

// 直接上传文件处理
// @Summary 直接上传文件
// @Description 通过临时令牌直接上传文件
// @Tags 存储服务
// @Accept multipart/form-data
// @Produce json
// @Param token formData string true "上传令牌"
// @Param file formData file true "文件"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /upload [post]
func DirectUploadHandler(c *gin.Context, storageService *service.StorageServiceImpl, cfg *config.Config) {
	// 获取并验证令牌
	token := c.PostForm("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少上传令牌"})
		return
	}

	// 解析JWT令牌
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Security.JWTSecret), nil
	})

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的令牌", "detail": err.Error()})
		return
	}

	// 验证令牌有效性
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌验证失败"})
		return
	}

	// 获取文件信息
	fileID, ok := claims["file_id"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌中缺少文件ID"})
		return
	}

	// 获取上传文件
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "获取上传文件失败", "detail": err.Error()})
		return
	}

	// 检查文件大小
	size, ok := claims["size"].(float64)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌中缺少文件大小"})
		return
	}

	if file.Size > int64(size) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件大小超过预期"})
		return
	}

	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), "direct_upload", fileID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建临时目录失败", "detail": err.Error()})
		return
	}

	// 保存文件到临时目录
	tempFilePath := filepath.Join(tempDir, file.Filename)
	if err := c.SaveUploadedFile(file, tempFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存文件失败", "detail": err.Error()})
		return
	}

	// 计算文件哈希
	hash, err := storageService.CalculateFileHash(tempFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "计算文件哈希失败", "detail": err.Error()})
		return
	}

	// 将文件移动到存储位置
	storagePath := filepath.Join(cfg.Storage.LocalDir, hash[0:2], hash[2:4], hash)
	if err := os.MkdirAll(filepath.Dir(storagePath), 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建存储目录失败", "detail": err.Error()})
		return
	}

	// 如果文件已存在，直接返回成功
	if _, err := os.Stat(storagePath); err == nil {
		// 清理临时文件
		os.RemoveAll(tempDir)
		c.JSON(http.StatusOK, gin.H{
			"message": "文件已存在，上传成功",
			"hash":    hash,
			"size":    file.Size,
		})
		return
	}

	// 移动文件到存储位置
	if err := os.Rename(tempFilePath, storagePath); err != nil {
		// 如果跨设备移动失败，尝试复制
		if strings.Contains(err.Error(), "cross-device link") {
			srcFile, err := os.Open(tempFilePath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "打开源文件失败", "detail": err.Error()})
				return
			}
			defer srcFile.Close()

			dstFile, err := os.Create(storagePath)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "创建目标文件失败", "detail": err.Error()})
				return
			}
			defer dstFile.Close()

			if _, err := io.Copy(dstFile, srcFile); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "复制文件失败", "detail": err.Error()})
				return
			}

			// 设置文件权限
			if err := os.Chmod(storagePath, 0644); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "设置文件权限失败", "detail": err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "移动文件失败", "detail": err.Error()})
			return
		}
	}

	// 清理临时目录
	os.RemoveAll(tempDir)

	c.JSON(http.StatusOK, gin.H{
		"message": "上传成功",
		"hash":    hash,
		"size":    file.Size,
	})
}

// 直接下载文件处理
// @Summary 直接下载文件
// @Description 通过临时令牌直接下载文件
// @Tags 存储服务
// @Accept json
// @Produce octet-stream
// @Param token query string true "下载令牌"
// @Success 200 {file} file
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /download [get]
func DirectDownloadHandler(c *gin.Context, storageService *service.StorageServiceImpl, cfg *config.Config) {
	// 获取并验证令牌
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少下载令牌"})
		return
	}

	// 解析JWT令牌
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Security.JWTSecret), nil
	})

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的令牌", "detail": err.Error()})
		return
	}

	// 验证令牌有效性
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok || !parsedToken.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "令牌验证失败"})
		return
	}

	// 获取文件哈希
	fileHash, ok := claims["file_hash"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌中缺少文件哈希"})
		return
	}

	// 获取文件名
	filename, ok := claims["filename"].(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "令牌中缺少文件名"})
		return
	}

	// 构建文件路径
	filePath := filepath.Join(cfg.Storage.LocalDir, fileHash[0:2], fileHash[2:4], fileHash)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 设置响应头
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")

	// 发送文件
	c.File(filePath)
}

// 注册HTTP路由
func RegisterHTTPHandlers(r *gin.Engine, storageService *service.StorageServiceImpl, cfg *config.Config) {
	// ... existing code ...

	// 添加直接上传和下载处理
	r.POST("/upload", func(c *gin.Context) {
		DirectUploadHandler(c, storageService, cfg)
	})

	r.GET("/download", func(c *gin.Context) {
		DirectDownloadHandler(c, storageService, cfg)
	})

	// ... existing code ...
}

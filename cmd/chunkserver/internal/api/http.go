package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cloudDrive/cmd/chunkserver/internal/config"
	"cloudDrive/cmd/chunkserver/internal/service"
	"cloudDrive/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
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

	// API路由组
	apiGroup := router.Group("/api")
	{
		// 健康检查端点
		apiGroup.GET("/health", handleHealthCheck(redis))

		// 文件操作API
		apiGroup.POST("/file/:id", handleFileUpload(service))
		apiGroup.GET("/file/:id", handleFileDownload(service))
		apiGroup.DELETE("/file/:id", handleFileDelete(service))

		// 分片上传API
		apiGroup.POST("/multipart/init", handleInitMultipart(service))
		apiGroup.POST("/multipart/part", handleUploadPart(service))
		apiGroup.GET("/multipart/status", handleMultipartStatus(service))
		apiGroup.POST("/multipart/complete", handleCompleteMultipart(service))
	}

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

// 健康检查处理函数
func handleHealthCheck(redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		health := map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		}

		// 检查Redis连接
		if redisClient != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			defer cancel()

			err := redisClient.Ping(ctx).Err()
			if err != nil {
				health["redis_status"] = "error"
				health["redis_error"] = err.Error()
				health["status"] = "degraded"
			} else {
				health["redis_status"] = "ok"
			}
		}

		if health["status"] == "ok" {
			c.JSON(http.StatusOK, health)
		} else {
			c.JSON(http.StatusServiceUnavailable, health)
		}
	}
}

// 处理文件上传
func handleFileUpload(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := c.Param("id")
		if fileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "文件ID不能为空",
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

// 处理文件下载
func handleFileDownload(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := c.Param("id")
		if fileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "文件ID不能为空",
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

// 处理文件删除
func handleFileDelete(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := c.Param("id")
		if fileID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "文件ID不能为空",
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
		// 支持两种请求方式：查询参数和JSON请求体
		var filename, token string
		var fileID string

		// 检查是否是JSON请求
		contentType := c.GetHeader("Content-Type")
		if strings.Contains(contentType, "application/json") {
			// 处理JSON请求体
			var req struct {
				FileID     string `json:"file_id"`
				Filename   string `json:"filename"`
				Name       string `json:"name"`
				Size       int64  `json:"size"`
				Hash       string `json:"hash"`
				TotalParts int    `json:"total_parts"`
				ParentID   string `json:"parent_id"`
			}

			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    1,
					"message": "无效的请求参数",
				})
				return
			}

			// 优先使用file_id和filename字段
			if req.FileID != "" {
				fileID = req.FileID
			} else {
				fileID = req.Hash // 兼容旧版本
			}

			if req.Filename != "" {
				filename = req.Filename
			} else {
				filename = req.Name // 兼容旧版本
			}

			// 对于JSON请求，我们不需要token，因为这是从主服务器发来的内部请求
			// 生成一个临时token
			token = fmt.Sprintf("internal_%s", fileID)

			// 将token保存到Redis，以便后续验证
			ctx := context.Background()
			tokenKey := fmt.Sprintf("chunk:token:%s", token)
			service.GetRedisClient().Set(ctx, tokenKey, fileID, 24*time.Hour)
		} else {
			// 处理查询参数
			filename = c.Query("filename")
			token = c.Query("token")

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
			var ok bool
			fileID, ok = tokenInfo["file_id"].(string)
			if !ok || fileID == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"code":    3,
					"message": "无效的文件ID",
				})
				return
			}
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

		// 获取服务器的公共URL
		// 优先从请求的Host头获取
		serverURL := c.Request.Header.Get("X-Forwarded-Host")
		if serverURL == "" {
			serverURL = c.Request.Host
		}

		// 构建完整的URL
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		publicURL := fmt.Sprintf("%s://%s", scheme, serverURL)

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "初始化成功",
			"data": gin.H{
				"upload_id":  uploadID,
				"server_url": publicURL, // 返回服务器URL
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

// 处理分片上传状态查询
func handleMultipartStatus(service *service.StorageServiceImpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadID := c.Query("upload_id")
		if uploadID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    1,
				"message": "upload_id参数必填",
			})
			return
		}

		// 获取已上传的分片
		parts, err := service.ListParts(c.Request.Context(), uploadID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    2,
				"message": fmt.Sprintf("获取分片状态失败: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "获取成功",
			"data": gin.H{
				"upload_id": uploadID,
				"parts":     parts,
			},
		})
	}
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

// 注册HTTP路由
func RegisterHTTPHandlers(r *gin.Engine, storageService *service.StorageServiceImpl, cfg *config.Config) {
	// ... existing code ...

	// 添加直接上传和下载处理
	r.POST("/upload", handleDirectUpload(storageService))
	r.GET("/download", handleDirectDownload(storageService))

	// ... existing code ...
}

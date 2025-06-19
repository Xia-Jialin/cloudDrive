package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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

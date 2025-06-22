package handler

import (
	"cloudDrive/internal/file"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// RedisInterface 定义Redis客户端接口
type RedisInterface interface {
	Ping(ctx context.Context) *redis.StatusCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Keys(ctx context.Context, pattern string) *redis.StringSliceCmd
	FlushDB(ctx context.Context) *redis.StatusCmd
}

type CreateFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

// @Summary 新建文件夹
// @Description 在指定目录下新建文件夹，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param data body handler.CreateFolderRequest true "文件夹信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /folders [post]
func CreateFolderHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件夹名不能为空"})
		return
	}
	folder, err := file.CreateFolder(db, req.Name, req.ParentID, userID)
	if err != nil {
		if err == file.ErrNameExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "同目录下已存在同名文件夹"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 创建文件夹成功后清理缓存
	redisClient := c.MustGet("redis")
	ctx := context.Background()

	// 尝试转换为具体的Redis客户端类型
	if rdb, ok := redisClient.(*redis.Client); ok {
		// 清理文件列表缓存（所有目录）
		fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
		keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
	} else if rdbInterface, ok := redisClient.(RedisInterface); ok {
		// 使用接口方法清理缓存
		fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
		keys, _ := rdbInterface.Keys(ctx, fileListPrefix+"*").Result()
		if len(keys) > 0 {
			rdbInterface.Del(ctx, keys...)
		}
	}

	c.JSON(http.StatusOK, gin.H{"id": folder.ID, "name": folder.Name})
}

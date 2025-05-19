package handler

import (
	"cloudDrive/internal/file"
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// GET /api/recycle
func RecycleBinListHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	page := 1
	pageSize := 20
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps := c.Query("page_size"); ps != "" {
		fmt.Sscanf(ps, "%d", &pageSize)
	}
	files, err := file.ListRecycleBinFiles(db, userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, files)
}

// POST /api/recycle/restore
func RecycleBinRestoreHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var req struct {
		FileID     string `json:"file_id" binding:"required"`
		TargetPath string `json:"target_path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	// 这里只实现基础还原，target_path逻辑可后续扩展
	err := file.RestoreFile(db, req.FileID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 清理文件列表缓存（所有目录）
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	c.JSON(http.StatusOK, gin.H{"message": "还原成功"})
}

// DELETE /api/recycle
func RecycleBinDeleteHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var req struct {
		FileID string `json:"file_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	err := file.PermanentlyDeleteFile(db, req.FileID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "彻底删除成功"})
}

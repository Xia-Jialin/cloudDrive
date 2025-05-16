package handler

import (
	"net/http"
	"time"

	"cloudDrive/internal/file"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PublicShareRequest 公开分享创建请求体
// resource_id: 被分享的文件/文件夹ID
// expire_hours: 分享有效期（小时）
type PublicShareRequest struct {
	ResourceID  string `json:"resource_id" binding:"required"`
	ExpireHours int    `json:"expire_hours" binding:"required,min=1,max=168"` // 1~168小时
}

// PublicShareResponse 公开分享创建响应体
type PublicShareResponse struct {
	ShareLink string `json:"share_link"`
	ExpireAt  int64  `json:"expire_at"`
}

// PublicShareAccessResponse 分享访问响应体
// 可根据需要返回更多资源信息
// 这里只返回文件基本信息

type PublicShareAccessResponse struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	OwnerID    uint   `json:"owner_id"`
	ExpireAt   int64  `json:"expire_at"`
}

// CreatePublicShareHandler 创建公开分享链接
func CreatePublicShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var req PublicShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}
	// 检查资源是否存在
	var f file.File
	if err := db.First(&f, "id = ?", req.ResourceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
		return
	}
	// 检查是否已有未过期的公开分享
	var existShare file.Share
	if err := db.Where("resource_id = ? AND share_type = ? AND expire_at > ?", req.ResourceID, "public", time.Now()).First(&existShare).Error; err == nil {
		shareLink := c.Request.Host + "/api/share/" + existShare.Token
		c.JSON(http.StatusOK, PublicShareResponse{
			ShareLink: shareLink,
			ExpireAt:  existShare.ExpireAt.Unix(),
		})
		return
	}
	// 生成唯一token
	token := uuid.New().String()
	expireAt := time.Now().Add(time.Duration(req.ExpireHours) * time.Hour)
	creatorID := f.OwnerID // 也可从登录态获取
	share := file.Share{
		ResourceID: req.ResourceID,
		ShareType:  "public",
		Token:      token,
		ExpireAt:   expireAt,
		CreatorID:  creatorID,
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&share).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分享失败"})
		return
	}
	// 返回分享链接
	shareLink := c.Request.Host + "/api/share/" + token
	c.JSON(http.StatusOK, PublicShareResponse{
		ShareLink: shareLink,
		ExpireAt:  expireAt.Unix(),
	})
}

// AccessPublicShareHandler 公开分享访问接口
func AccessPublicShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	token := c.Param("token")
	var share file.Share
	if err := db.Where("token = ? AND share_type = ?", token, "public").First(&share).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享链接不存在"})
		return
	}
	if time.Now().After(share.ExpireAt) {
		c.JSON(http.StatusGone, gin.H{"error": "分享链接已过期"})
		return
	}
	// 查找资源信息
	var f file.File
	if err := db.First(&f, "id = ?", share.ResourceID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
		return
	}
	c.JSON(http.StatusOK, PublicShareAccessResponse{
		ResourceID: f.ID,
		Name:       f.Name,
		Type:       f.Type,
		OwnerID:    f.OwnerID,
		ExpireAt:   share.ExpireAt.Unix(),
	})
}

// ShareDownloadHandler 公开分享文件下载接口
// GET /api/share/download/:token
func ShareDownloadHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	token := c.Param("token")
	var share file.Share
	if err := db.Where("token = ? AND share_type = ?", token, "public").First(&share).Error; err != nil {
		c.JSON(404, gin.H{"error": "分享链接不存在"})
		return
	}
	if time.Now().After(share.ExpireAt) {
		c.JSON(410, gin.H{"error": "分享链接已过期"})
		return
	}
	var f file.File
	if err := db.First(&f, "id = ?", share.ResourceID).Error; err != nil {
		c.JSON(404, gin.H{"error": "文件不存在"})
		return
	}
	if f.Type != "file" {
		c.JSON(400, gin.H{"error": "只能下载文件类型"})
		return
	}
	filePath := "uploads/" + f.Hash
	c.FileAttachment(filePath, f.Name)
}

// GetPublicShareHandler 查询已有未过期的公开分享
func GetPublicShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	resourceID := c.Query("resource_id")
	if resourceID == "" {
		c.JSON(400, gin.H{"error": "resource_id参数必填"})
		return
	}
	var share file.Share
	if err := db.Where("resource_id = ? AND share_type = ? AND expire_at > ?", resourceID, "public", time.Now()).First(&share).Error; err != nil {
		c.JSON(404, gin.H{"error": "未找到分享链接"})
		return
	}
	shareLink := c.Request.Host + "/api/share/" + share.Token
	c.JSON(200, PublicShareResponse{
		ShareLink: shareLink,
		ExpireAt:  share.ExpireAt.Unix(),
	})
}

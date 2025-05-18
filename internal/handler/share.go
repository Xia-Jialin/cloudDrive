package handler

import (
	"net/http"
	"time"

	"cloudDrive/internal/file"

	"math/rand"

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

// PrivateShareRequest 私有分享创建请求体
type PrivateShareRequest struct {
	ResourceID  string `json:"resource_id" binding:"required"`
	ExpireHours int    `json:"expire_hours" binding:"required,min=1,max=168"`
}

type PrivateShareResponse struct {
	ShareLink  string `json:"share_link"`
	AccessCode string `json:"access_code"`
	ExpireAt   int64  `json:"expire_at"`
}

// 生成4位字母数字混合码
func genAccessCode() string {
	letters := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune, 4)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// @Summary 创建公开分享链接
// @Description 创建一个公开分享链接，任何人可访问
// @Tags 分享
// @Accept json
// @Produce json
// @Param data body handler.PublicShareRequest true "公开分享参数"
// @Success 200 {object} handler.PublicShareResponse
// @Failure 400 {object} map[string]interface{}
// @Router /share/public [post]
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

// @Summary 分享文件下载
// @Description 通过分享链接下载文件，私有需access_code
// @Tags 分享
// @Accept json
// @Produce octet-stream
// @Param token path string true "分享Token"
// @Param access_code query string false "访问码(私有分享)"
// @Success 200 {file} file
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 410 {object} map[string]interface{}
// @Router /share/download/{token} [get]
func ShareDownloadHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	token := c.Param("token")
	var share file.Share
	if err := db.Where("token = ?", token).First(&share).Error; err != nil {
		c.JSON(404, gin.H{"error": "分享链接不存在"})
		return
	}
	if time.Now().After(share.ExpireAt) {
		c.JSON(410, gin.H{"error": "分享链接已过期"})
		return
	}
	if share.ShareType == "private" {
		accessCode := c.Query("access_code")
		if accessCode == "" {
			c.JSON(401, gin.H{"error": "需要访问码"})
			return
		}
		if accessCode != share.AccessCode {
			c.JSON(403, gin.H{"error": "访问码错误"})
			return
		}
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

// @Summary 查询已有未过期的公开分享
// @Description 查询指定文件的未过期公开分享链接
// @Tags 分享
// @Accept json
// @Produce json
// @Param resource_id query string true "资源ID"
// @Success 200 {object} handler.PublicShareResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /share/public [get]
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

// @Summary 创建私有分享链接
// @Description 创建一个私有分享链接，需访问码访问
// @Tags 分享
// @Accept json
// @Produce json
// @Param data body handler.PrivateShareRequest true "私有分享参数"
// @Success 200 {object} handler.PrivateShareResponse
// @Failure 400 {object} map[string]interface{}
// @Router /share/private [post]
func CreatePrivateShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var req PrivateShareRequest
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
	// 生成唯一token和访问码
	token := uuid.New().String()
	accessCode := genAccessCode()
	expireAt := time.Now().Add(time.Duration(req.ExpireHours) * time.Hour)
	creatorID := f.OwnerID
	share := file.Share{
		ResourceID: req.ResourceID,
		ShareType:  "private",
		Token:      token,
		AccessCode: accessCode,
		ExpireAt:   expireAt,
		CreatorID:  creatorID,
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&share).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分享失败"})
		return
	}
	shareLink := c.Request.Host + "/api/share/" + token
	c.JSON(http.StatusOK, PrivateShareResponse{
		ShareLink:  shareLink,
		AccessCode: accessCode,
		ExpireAt:   expireAt.Unix(),
	})
}

// @Summary 访问分享链接
// @Description 访问公开或私有分享链接，私有需access_code
// @Tags 分享
// @Accept json
// @Produce json
// @Param token path string true "分享Token"
// @Param access_code query string false "访问码(私有分享)"
// @Success 200 {object} handler.PublicShareAccessResponse
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 410 {object} map[string]interface{}
// @Router /share/{token} [get]
func AccessShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	token := c.Param("token")
	var share file.Share
	if err := db.Where("token = ?", token).First(&share).Error; err != nil {
		c.JSON(404, gin.H{"error": "分享链接不存在"})
		return
	}
	if time.Now().After(share.ExpireAt) {
		c.JSON(410, gin.H{"error": "分享链接已过期"})
		return
	}
	if share.ShareType == "private" {
		accessCode := c.Query("access_code")
		if accessCode == "" {
			c.JSON(401, gin.H{"error": "需要访问码"})
			return
		}
		if accessCode != share.AccessCode {
			c.JSON(403, gin.H{"error": "访问码错误"})
			return
		}
	}
	// 查找资源信息
	var f file.File
	if err := db.First(&f, "id = ?", share.ResourceID).Error; err != nil {
		c.JSON(404, gin.H{"error": "资源不存在"})
		return
	}
	c.JSON(200, PublicShareAccessResponse{
		ResourceID: f.ID,
		Name:       f.Name,
		Type:       f.Type,
		OwnerID:    f.OwnerID,
		ExpireAt:   share.ExpireAt.Unix(),
	})
}

// @Summary 查询已有未过期的私有分享
// @Description 查询指定文件的未过期私有分享链接
// @Tags 分享
// @Accept json
// @Produce json
// @Param resource_id query string true "资源ID"
// @Success 200 {object} handler.PrivateShareResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /share/private [get]
func GetPrivateShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	resourceID := c.Query("resource_id")
	if resourceID == "" {
		c.JSON(400, gin.H{"error": "resource_id参数必填"})
		return
	}
	var share file.Share
	if err := db.Where("resource_id = ? AND share_type = ? AND expire_at > ?", resourceID, "private", time.Now()).First(&share).Error; err != nil {
		c.JSON(404, gin.H{"error": "未找到私有分享链接"})
		return
	}
	shareLink := c.Request.Host + "/api/share/" + share.Token
	c.JSON(200, PrivateShareResponse{
		ShareLink:  shareLink,
		AccessCode: share.AccessCode,
		ExpireAt:   share.ExpireAt.Unix(),
	})
}

// @Summary 取消分享
// @Description 取消指定的分享链接（token或resource_id），仅分享创建者可操作，需登录（Session）
// @Tags 分享
// @Accept json
// @Produce json
// @Param token path string false "分享Token"
// @Param resource_id query string false "资源ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /share [delete]
func CancelShareHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	token := c.Query("token")
	resourceID := c.Query("resource_id")
	if token == "" && resourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token或resource_id参数必填"})
		return
	}
	var share file.Share
	var dbErr error
	if token != "" {
		dbErr = db.Where("token = ?", token).First(&share).Error
	} else {
		dbErr = db.Where("resource_id = ? AND creator_id = ? AND expire_at > ?", resourceID, userID, time.Now()).First(&share).Error
	}
	if dbErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分享不存在"})
		return
	}
	if share.CreatorID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限取消该分享"})
		return
	}
	if err := db.Delete(&share).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消分享失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "取消分享成功"})
}

package handler

import (
	"cloudDrive/internal/user"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterHandler 用户注册
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
	resp, err := user.Register(c.MustGet("db").(*gorm.DB), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": resp.ID})
}

// LoginHandler 用户登录
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
	resp, err := user.Login(c.MustGet("db").(*gorm.DB), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 登录成功，写入session
	session := sessions.Default(c)
	session.Set("user_id", resp.User.ID)
	session.Save()
	c.JSON(http.StatusOK, gin.H{"user": resp.User})
}

// LogoutHandler 用户退出登录
// @Summary 用户退出登录
// @Description 退出登录，清除session
// @Tags 用户模块
// @Success 200 {object} map[string]interface{}
// @Router /user/logout [post]
func LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{
		MaxAge: -1, // 立即过期
		Path:   "/",
	})
	session.Delete("user_id")
	session.Save()
	c.JSON(http.StatusOK, gin.H{"msg": "已退出登录"})
}

// @Summary 获取用户存储空间信息
// @Description 获取当前用户的存储空间使用量和总量，需登录（Session）
// @Tags 用户模块
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/storage [get]
func UserStorageHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var u user.User
	if err := db.First(&u, userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storage_used": u.StorageUsed, "storage_limit": u.StorageLimit})
}

// @Summary 获取当前用户信息
// @Description 获取当前登录用户的基本信息
// @Tags 用户模块
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /user/me [get]
func UserMeHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var u user.User
	if err := db.First(&u, userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}
	u.Password = "" // 不返回密码
	c.JSON(http.StatusOK, gin.H{"user": u})
}

package handler

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/user"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type CreateFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

// @Summary 新建文件夹
// @Description 在指定目录下新建文件夹
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer Token"
// @Param data body handler.CreateFolderRequest true "文件夹信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /folders [post]
func CreateFolderHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
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
	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件夹名不能为空"})
		return
	}
	folder, err := file.CreateFolder(db, req.Name, req.ParentID, claims.UserID)
	if err != nil {
		if err == file.ErrNameExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "同目录下已存在同名文件夹"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": folder.ID, "name": folder.Name})
}

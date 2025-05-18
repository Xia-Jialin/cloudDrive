package handler

import (
	"cloudDrive/internal/file"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	c.JSON(http.StatusOK, gin.H{"id": folder.ID, "name": folder.Name})
}

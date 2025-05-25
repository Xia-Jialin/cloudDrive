package handler

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/storage"
	"cloudDrive/internal/user"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// @Summary 获取文件/文件夹列表
// @Description 获取指定目录下的文件和文件夹，支持分页和排序，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param parent_id query string false "父目录ID，根目录为0"
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Param order_by query string false "排序字段，默认upload_time"
// @Param order query string false "排序方式，asc/desc，默认desc"
// @Success 200 {object} file.ListFilesResponse
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files [get]
func FileListHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	rdb := c.MustGet("redis").(*redis.Client)
	userID := c.MustGet("user_id").(uint)
	parentID := c.DefaultQuery("parent_id", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	orderBy := c.DefaultQuery("order_by", "upload_time")
	order := c.DefaultQuery("order", "desc")

	// 构造缓存key
	cacheKey := fmt.Sprintf("filelist:%d:%s:%d:%d:%s:%s", userID, parentID, page, pageSize, orderBy, order)
	ctx := context.Background()
	if val, err := rdb.Get(ctx, cacheKey).Result(); err == nil && val != "" {
		c.Data(http.StatusOK, "application/json", []byte(val))
		return
	}

	resp, err := file.ListFiles(db, file.ListFilesRequest{
		ParentID: parentID,
		OwnerID:  userID,
		Page:     page,
		PageSize: pageSize,
		OrderBy:  orderBy,
		Order:    order,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filesWithSize := make([]gin.H, 0, len(resp.Files))
	for _, f := range resp.Files {
		fileMap := gin.H{
			"id":          f.ID,
			"name":        f.Name,
			"type":        f.Type,
			"parent_id":   f.ParentID,
			"owner_id":    f.OwnerID,
			"upload_time": f.UploadTime,
		}
		if f.Type == "file" {
			var fc file.FileContent
			db.First(&fc, "hash = ?", f.Hash)
			fileMap["size"] = fc.Size
		} else {
			fileMap["size"] = nil
		}
		filesWithSize = append(filesWithSize, fileMap)
	}
	result := gin.H{"files": filesWithSize, "total": resp.Total}
	jsonBytes, _ := json.Marshal(result)
	// 写入缓存
	rdb.Set(ctx, cacheKey, jsonBytes, 5*time.Minute)
	c.Data(http.StatusOK, "application/json", jsonBytes)
}

// StorageKey 用于gin.Context注入storage依赖
const StorageKey = "storage"

// Storage实例获取函数（可根据实际情况注入或配置）
func getStorage() storage.Storage {
	return &storage.LocalFileStorage{Dir: "uploads"}
}

// @Summary 上传文件
// @Description 上传文件到指定目录，需登录（Session）
// @Tags 文件模块
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "文件"
// @Param parent_id formData string false "父目录ID，根目录为0"
// @Param hash formData string false "前端计算的文件hash"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/upload [post]
func FileUploadHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	stor := c.MustGet(StorageKey).(storage.Storage)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未选择文件"})
		return
	}
	parentID := c.PostForm("parent_id")
	if parentID == "" {
		var userRoot file.UserRoot
		if err := db.First(&userRoot, "user_id = ?", userID).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查找根目录失败", "detail": err.Error()})
			return
		}
		parentID = userRoot.RootID
	}
	fileObj, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件读取失败", "detail": err.Error()})
		return
	}
	defer fileObj.Close()
	clientHash := c.PostForm("hash")
	hashObj := sha256.New()
	tmpFilePath := "tmp_upload_" + uuid.New().String()
	tmpFile, err := os.Create(tmpFilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "临时文件创建失败", "detail": err.Error()})
		return
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFilePath)
	}()
	tee := io.TeeReader(fileObj, hashObj)
	_, err = io.Copy(tmpFile, tee)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件写入失败", "detail": err.Error()})
		return
	}
	serverHash := hex.EncodeToString(hashObj.Sum(nil))
	if clientHash != "" && clientHash != serverHash {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件内容校验失败，请重试"})
		return
	}
	hashStr := serverHash
	// 检查 hash 是否已存在
	var fileContent file.FileContent
	err = db.First(&fileContent, "hash = ?", hashStr).Error
	if err == gorm.ErrRecordNotFound {
		tmpFile.Seek(0, 0)
		err = stor.Save(hashStr, tmpFile)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败", "detail": err.Error()})
			return
		}
		fileContent = file.FileContent{
			Hash: hashStr,
			Size: fileHeader.Size,
		}
		err = db.Create(&fileContent).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
			return
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败", "detail": err.Error()})
		return
	}
	f := file.File{
		Name:       fileHeader.Filename,
		Hash:       hashStr,
		Type:       "file",
		ParentID:   parentID,
		OwnerID:    userID,
		UploadTime: time.Now(),
	}
	if err := db.Create(&f).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
		return
	}

	u, err := user.GetUserByID(db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "用户不存在"})
		return
	}
	if u.StorageUsed+fileContent.Size > u.StorageLimit {
		if err := db.Delete(&f).Error; err == nil {
			_ = stor.Delete(hashStr)
		}
		c.JSON(http.StatusForbidden, gin.H{"error": "存储空间不足"})
		return
	}
	if err := user.UpdateUserStorageUsed(db, userID, fileContent.Size); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新存储空间失败", "detail": err.Error()})
		return
	}
	// 清理用户缓存
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user:info:%d", userID)
	rdb.Del(ctx, cacheKey)
	// 清理文件列表缓存（父目录）
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	c.JSON(http.StatusOK, gin.H{"id": f.ID, "name": f.Name, "size": fileContent.Size})
}

// @Summary 下载文件
// @Description 下载指定文件，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "文件ID"
// @Success 200 {file} file
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/download/{id} [get]
func FileDownloadHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	rdb := c.MustGet("redis").(*redis.Client)
	userID := c.MustGet("user_id").(uint)
	idStr := c.Param("id")
	ctx := context.Background()
	cacheKey := fmt.Sprintf("filemeta:%s", idStr)
	var f file.File
	if val, err := rdb.Get(ctx, cacheKey).Result(); err == nil && val != "" {
		if err := json.Unmarshal([]byte(val), &f); err == nil {
			if f.OwnerID != userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权限下载该文件"})
				return
			}
			if f.Type != "file" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "只能下载文件类型"})
				return
			}
			filePath := "uploads/" + f.Hash
			c.FileAttachment(filePath, f.Name)
			return
		}
	}
	err := db.First(&f, "id = ?", idStr).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}
	if f.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限下载该文件"})
		return
	}
	if f.Type != "file" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能下载文件类型"})
		return
	}
	jsonBytes, _ := json.Marshal(f)
	rdb.Set(ctx, cacheKey, jsonBytes, 5*time.Minute)
	filePath := "uploads/" + f.Hash
	c.FileAttachment(filePath, f.Name)
}

// @Summary 删除文件
// @Description 删除指定文件，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param id path string true "文件ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/{id} [delete]
func FileDeleteHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	idStr := c.Param("id")
	err := file.DeleteFile(db, idStr, userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		if err == file.ErrNoPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限删除该文件"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
	// 清理用户缓存
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user:info:%d", userID)
	rdb.Del(ctx, cacheKey)
	// 清理文件列表缓存（所有目录）
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	// 清理文件元数据缓存
	fileMetaKey := fmt.Sprintf("filemeta:%s", idStr)
	rdb.Del(ctx, fileMetaKey)
}

// @Summary 重命名文件
// @Description 重命名指定文件，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param id path string true "文件ID"
// @Param data body file.RenameFileRequest true "新文件名"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/{id}/rename [put]
func FileRenameHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	idStr := c.Param("id")
	var req file.RenameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.NewName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新文件名不能为空"})
		return
	}
	err := file.RenameFile(db, idStr, userID, req.NewName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		if err == file.ErrNoPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限重命名该文件"})
			return
		}
		if err == file.ErrNameExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "同目录下已存在同名文件"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "重命名成功"})
	// 清理文件列表缓存（所有目录）
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	// 清理文件元数据缓存
	fileMetaKey := fmt.Sprintf("filemeta:%s", idStr)
	rdb.Del(ctx, fileMetaKey)
}

// @Summary 移动文件/文件夹
// @Description 移动指定文件/文件夹到新目录，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param id path string true "文件ID"
// @Param data body file.MoveFileRequest true "新父目录ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/{id}/move [put]
func FileMoveHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	idStr := c.Param("id")
	var req file.MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新父目录ID不能为空"})
		return
	}
	err := file.MoveFile(db, idStr, userID, req.NewParentID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
			return
		}
		if err == file.ErrNoPermission {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权限移动该文件"})
			return
		}
		if err == file.ErrNameExists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "目标目录下已存在同名文件/文件夹"})
			return
		}
		if err == file.ErrMoveToSelfOrChild {
			c.JSON(http.StatusBadRequest, gin.H{"error": "不能移动到自身或子目录下"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "移动成功"})
	// 清理文件列表缓存（所有目录）
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	// 清理文件元数据缓存
	fileMetaKey := fmt.Sprintf("filemeta:%s", idStr)
	rdb.Del(ctx, fileMetaKey)
}

// @Summary 搜索文件
// @Description 按文件名模糊搜索文件，需登录（Session）
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param name query string true "文件名"
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Success 200 {object} file.ListFilesResponse
// @Router /files/search [get]
func FileSearchHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	name := c.Query("name")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	resp, err := file.ListFiles(db, file.ListFilesRequest{
		OwnerID:  userID,
		Name:     name,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filesWithSize := make([]gin.H, 0, len(resp.Files))
	for _, f := range resp.Files {
		fileMap := gin.H{
			"id":          f.ID,
			"name":        f.Name,
			"type":        f.Type,
			"parent_id":   f.ParentID,
			"owner_id":    f.OwnerID,
			"upload_time": f.UploadTime,
		}
		if f.Type == "file" {
			var fc file.FileContent
			db.First(&fc, "hash = ?", f.Hash)
			fileMap["size"] = fc.Size
		} else {
			fileMap["size"] = nil
		}
		filesWithSize = append(filesWithSize, fileMap)
	}
	c.JSON(http.StatusOK, gin.H{"files": filesWithSize, "total": resp.Total})
}

// @Summary 文件在线预览
// @Description 在线预览指定文件，仅支持已登录用户
// @Tags 文件模块
// @Accept json
// @Produce octet-stream
// @Param id path string true "文件ID"
// @Success 200 {file} file
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/preview/{id} [get]
func FilePreviewHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	rdb := c.MustGet("redis").(*redis.Client)
	userID := c.MustGet("user_id").(uint)
	idStr := c.Param("id")
	ctx := context.Background()
	cacheKey := fmt.Sprintf("filemeta:%s", idStr)
	var f file.File
	if val, err := rdb.Get(ctx, cacheKey).Result(); err == nil && val != "" {
		if err := json.Unmarshal([]byte(val), &f); err == nil {
			if f.OwnerID != userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "无权限预览该文件"})
				return
			}
			if f.Type != "file" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "只能预览文件类型"})
				return
			}
			filePath := "uploads/" + f.Hash
			ext := ""
			if len(f.Name) > 0 {
				dot := len(f.Name) - 1
				for ; dot >= 0 && f.Name[dot] != '.'; dot-- {
				}
				if dot >= 0 {
					ext = f.Name[dot:]
				}
			}
			contentType := "application/octet-stream"
			if ext != "" {
				contentType = mime.TypeByExtension(ext)
				if contentType == "" {
					contentType = "application/octet-stream"
				}
			}
			c.Header("Content-Type", contentType)
			c.File(filePath)
			return
		}
	}
	err := db.First(&f, "id = ?", idStr).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}
	if f.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限预览该文件"})
		return
	}
	if f.Type != "file" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能预览文件类型"})
		return
	}
	jsonBytes, _ := json.Marshal(f)
	rdb.Set(ctx, cacheKey, jsonBytes, 5*time.Minute)
	filePath := "uploads/" + f.Hash
	ext := ""
	if len(f.Name) > 0 {
		dot := len(f.Name) - 1
		for ; dot >= 0 && f.Name[dot] != '.'; dot-- {
		}
		if dot >= 0 {
			ext = f.Name[dot:]
		}
	}
	contentType := "application/octet-stream"
	if ext != "" {
		contentType = mime.TypeByExtension(ext)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	c.Header("Content-Type", contentType)
	c.File(filePath)
}

package handler

import (
	"bytes"
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

// StorageKey 用于在gin上下文中获取存储服务实例的键
const StorageKey = "storage"

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

// Storage实例获取函数（可根据实际情况注入或配置）
func getStorage() storage.Storage {
	// 这里应该从配置中获取存储服务的类型和配置
	// 由于我们没有Redis客户端，这里返回一个LocalFileStorage实例
	// 在实际应用中，应该使用正确配置的ChunkServerStorage
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
		err = stor.Upload(context.Background(), hashStr, tmpFile)
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
			_ = stor.Delete(context.Background(), hashStr)
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

// ================= 分片上传相关接口（含Redis校验） =================

// 工具函数：校验 uploadId 是否属于当前用户，并返回上传信息
func checkUploadIdBelongsToUser(c *gin.Context, uploadId string) (map[string]interface{}, bool) {
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	userID := c.MustGet("user_id").(uint)
	val, err := rdb.Get(ctx, "upload:"+uploadId).Result()
	if err != nil {
		return nil, false
	}
	var info map[string]interface{}
	if err := json.Unmarshal([]byte(val), &info); err != nil {
		return nil, false
	}
	uid, ok := info["user_id"].(float64)
	if !ok || uint(uid) != userID {
		return nil, false
	}
	return info, true
}

// @Summary 初始化分片上传
// @Description 初始化分片上传，返回uploadId
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param name body string true "文件名"
// @Param size body int64 true "文件大小"
// @Param hash body string true "文件hash"
// @Param total_parts body int true "总分片数"
// @Success 200 {object} map[string]interface{}
// @Router /files/multipart/init [post]
func MultipartInitHandler(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		Hash       string `json:"hash"`
		TotalParts int    `json:"total_parts"`
		ParentID   string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.Hash == "" || req.TotalParts <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	// 秒传判断
	var fileContent file.FileContent
	err := db.First(&fileContent, "hash = ?", req.Hash).Error
	if err == nil {
		// 已存在，直接返回
		c.JSON(http.StatusOK, gin.H{
			"upload_id": "",
			"instant":   true,
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		// 只有当错误不是"记录未找到"时才返回错误
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文件内容失败", "detail": err.Error()})
		return
	}
	// 正常分片上传流程
	stor := c.MustGet(StorageKey).(storage.Storage)
	uploadId, err := stor.InitMultipartUpload(context.Background(), req.Hash, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化分片上传失败", "detail": err.Error()})
		return
	}

	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	info := map[string]interface{}{
		"user_id":     userID,
		"hash":        req.Hash,
		"total_parts": req.TotalParts,
		"size":        req.Size,
		"name":        req.Name,
		"parent_id":   req.ParentID,
	}
	infoJson, _ := json.Marshal(info)
	rdb.Set(ctx, "upload:"+uploadId, infoJson, 24*time.Hour)
	c.JSON(http.StatusOK, gin.H{"upload_id": uploadId, "instant": false})
}

// @Summary 上传分片
// @Description 上传单个分片
// @Tags 文件模块
// @Accept multipart/form-data
// @Produce json
// @Param upload_id formData string true "分片上传ID"
// @Param part_number formData int true "分片序号"
// @Param part file true "分片内容"
// @Success 200 {object} map[string]interface{}
// @Router /files/multipart/upload [post]
func MultipartUploadPartHandler(c *gin.Context) {
	stor := c.MustGet(StorageKey).(storage.Storage)
	uploadId := c.PostForm("upload_id")
	partNumberStr := c.PostForm("part_number")
	partNumber, err := strconv.Atoi(partNumberStr)
	if err != nil || uploadId == "" || partNumber <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	// 校验 uploadId 归属
	info, ok := checkUploadIdBelongsToUser(c, uploadId)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限或uploadId无效"})
		return
	}

	// 生成用于分片上传的token
	var token string
	chunkStorage, ok := stor.(*storage.ChunkServerStorage)
	if ok {
		// 准备上传信息
		uploadInfo := map[string]interface{}{
			"file_id":  info["hash"],
			"user_id":  c.MustGet("user_id").(uint),
			"filename": info["name"],
			"size":     info["size"],
		}

		// 生成token
		token, err = chunkStorage.GenerateUploadToken(uploadInfo, 3600) // 1小时过期
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "生成上传令牌失败", "detail": err.Error()})
			return
		}
	}

	fileHeader, err := c.FormFile("part")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未选择分片文件"})
		return
	}
	fileObj, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片读取失败"})
		return
	}
	defer fileObj.Close()
	data, err := io.ReadAll(fileObj)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片读取失败"})
		return
	}

	// 使用新的接口，传递token作为可选参数
	_, err = stor.UploadPart(context.Background(), uploadId, partNumber, bytes.NewReader(data), token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "分片保存失败", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "分片上传成功"})
}

// @Summary 查询已上传分片
// @Description 查询已上传分片序号
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param upload_id query string true "分片上传ID"
// @Success 200 {object} map[string]interface{}
// @Router /files/multipart/status [get]
func MultipartStatusHandler(c *gin.Context) {
	stor := c.MustGet(StorageKey).(storage.Storage)
	uploadId := c.Query("upload_id")
	if uploadId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	_, ok := checkUploadIdBelongsToUser(c, uploadId)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限或uploadId无效"})
		return
	}
	parts, err := stor.ListUploadedParts(context.Background(), uploadId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uploaded_parts": parts})
}

// @Summary 合并分片
// @Description 合并所有分片为完整文件
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param upload_id body string true "分片上传ID"
// @Param total_parts body int true "总分片数"
// @Param target_key body string true "合并后目标文件key（如hash）"
// @Success 200 {object} map[string]interface{}
// @Router /files/multipart/complete [post]
func MultipartCompleteHandler(c *gin.Context) {
	stor := c.MustGet(StorageKey).(storage.Storage)
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	var req struct {
		UploadId   string `json:"upload_id"`
		TotalParts int    `json:"total_parts"`
		TargetKey  string `json:"target_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UploadId == "" || req.TotalParts <= 0 || req.TargetKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	info, ok := checkUploadIdBelongsToUser(c, req.UploadId)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限或uploadId无效"})
		return
	}
	if int(info["total_parts"].(float64)) != req.TotalParts {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分片数不一致"})
		return
	}
	// ====== 空间配额校验 ======
	fileSize := int64(info["size"].(float64))
	u, err := user.GetUserByID(db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "用户不存在"})
		return
	}
	if u.StorageUsed+fileSize > u.StorageLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "存储空间不足"})
		return
	}
	// ========================
	// 生成分片信息列表
	parts := make([]storage.PartInfo, req.TotalParts)
	for i := 0; i < req.TotalParts; i++ {
		parts[i] = storage.PartInfo{
			PartNumber: i + 1,
			ETag:       "", // 这里可能需要实际的ETag值，但我们暂时不需要
		}
	}
	_, err = stor.CompleteMultipartUpload(context.Background(), req.UploadId, parts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "合并失败", "detail": err.Error()})
		return
	}

	// 注意：ChunkServer已经处理了文件合并和验证，这里不需要再次进行本地文件哈希校验
	// 因为文件存储在ChunkServer的存储系统中（MinIO或本地存储），而不是API Server的本地文件系统

	// 合并成功后更新用户已用空间
	if err := user.UpdateUserStorageUsed(db, userID, fileSize); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新存储空间失败", "detail": err.Error()})
		return
	}
	// 合并成功后插入 file_content 和 file 表
	hash := info["hash"].(string)
	name := info["name"].(string)
	parentID := c.DefaultQuery("parent_id", "")
	if parentID == "" {
		var userRoot file.UserRoot
		if err := db.First(&userRoot, "user_id = ?", userID).Error; err == nil {
			parentID = userRoot.RootID
		}
	}
	var fileContent file.FileContent
	err = db.First(&fileContent, "hash = ?", hash).Error
	if err == gorm.ErrRecordNotFound {
		// 记录不存在，创建新记录
		fileContent = file.FileContent{Hash: hash, Size: fileSize}
		if err := db.Create(&fileContent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建文件内容记录失败", "detail": err.Error()})
			return
		}
	} else if err != nil {
		// 其他错误
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询文件内容失败", "detail": err.Error()})
		return
	}
	f := file.File{
		Name:       name,
		Hash:       hash,
		Type:       "file",
		ParentID:   parentID,
		OwnerID:    userID,
		UploadTime: time.Now(),
	}
	db.Create(&f)
	// 合并成功后清理 Redis 记录
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	rdb.Del(ctx, "upload:"+req.UploadId)
	// 清理用户缓存
	cacheKey := fmt.Sprintf("user:info:%d", userID)
	rdb.Del(ctx, cacheKey)
	// 清理文件列表缓存（所有目录）
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}
	c.JSON(http.StatusOK, gin.H{"message": "合并成功"})
}

// @Summary 刷新分片上传令牌
// @Description 刷新分片上传的令牌，用于处理令牌过期的情况
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param data body map[string]string true "包含upload_id和hash的JSON"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /files/multipart/refresh-token [post]
func MultipartRefreshTokenHandler(c *gin.Context) {
	var req struct {
		UploadID string `json:"upload_id"`
		Hash     string `json:"hash"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UploadID == "" || req.Hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 校验 uploadId 归属
	info, ok := checkUploadIdBelongsToUser(c, req.UploadID)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限或uploadId无效"})
		return
	}

	// 生成新的令牌
	stor := c.MustGet(StorageKey).(storage.Storage)
	chunkStorage, ok := stor.(*storage.ChunkServerStorage)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "当前存储模式不支持分片上传"})
		return
	}

	// 准备上传信息
	uploadInfo := map[string]interface{}{
		"file_id":  info["hash"],
		"user_id":  c.MustGet("user_id").(uint),
		"filename": info["name"],
		"size":     info["size"],
	}

	// 生成新的token，有效期1小时
	token, err := chunkStorage.GenerateUploadToken(uploadInfo, 3600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成上传令牌失败", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// @Summary 获取临时上传URL
// @Description 获取临时上传URL，前端可直接与存储服务通信
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param parent_id query string false "父目录ID，根目录为空"
// @Param filename query string true "文件名"
// @Param size query int64 true "文件大小"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/upload-url [get]
func GetUploadURLHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	stor := c.MustGet(StorageKey).(storage.Storage)

	filename := c.Query("filename")
	sizeStr := c.Query("size")
	parentID := c.DefaultQuery("parent_id", "")

	if filename == "" || sizeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名和大小参数必填"})
		return
	}

	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件大小格式错误"})
		return
	}

	// 检查用户存储空间
	u, err := user.GetUserByID(db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户信息失败"})
		return
	}

	if u.StorageUsed+size > u.StorageLimit {
		c.JSON(http.StatusForbidden, gin.H{"error": "存储空间不足"})
		return
	}

	// 生成唯一的文件ID
	fileID := uuid.New().String()

	// 准备上传信息
	uploadInfo := map[string]interface{}{
		"user_id":   userID,
		"file_id":   fileID,
		"filename":  filename,
		"size":      size,
		"parent_id": parentID,
	}

	// 如果存储服务是ChunkServerStorage类型，生成临时上传URL
	chunkStorage, ok := stor.(*storage.ChunkServerStorage)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "当前存储模式不支持直接上传"})
		return
	}

	// 生成上传令牌，有效期30分钟
	token, err := chunkStorage.GenerateUploadToken(uploadInfo, 30*60)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成上传令牌失败"})
		return
	}

	// 将上传信息保存到Redis，用于上传完成后的处理
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	infoJson, _ := json.Marshal(uploadInfo)
	rdb.Set(ctx, "pending_upload:"+fileID, infoJson, 24*time.Hour)

	// 返回临时上传URL和令牌
	c.JSON(http.StatusOK, gin.H{
		"upload_url": fmt.Sprintf("%s/upload", chunkStorage.GetBaseURL()),
		"token":      token,
		"file_id":    fileID,
	})
}

// @Summary 获取临时下载URL
// @Description 获取临时下载URL，前端可直接与存储服务通信
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param id path string true "文件ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/download-url/{id} [get]
func GetDownloadURLHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)
	stor := c.MustGet(StorageKey).(storage.Storage)

	idStr := c.Param("id")

	// 查询文件信息
	var f file.File
	err := db.First(&f, "id = ?", idStr).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 检查权限
	if f.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限下载该文件"})
		return
	}

	// 如果不是文件类型
	if f.Type != "file" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能下载文件类型"})
		return
	}

	// 如果存储服务是ChunkServerStorage类型，生成临时下载URL
	chunkStorage, ok := stor.(*storage.ChunkServerStorage)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "当前存储模式不支持直接下载"})
		return
	}

	// 生成下载令牌，有效期15分钟
	token, err := chunkStorage.GenerateDownloadToken(f.Hash, f.Name, 15*60)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成下载令牌失败"})
		return
	}

	// 返回临时下载URL和令牌
	c.JSON(http.StatusOK, gin.H{
		"download_url": fmt.Sprintf("%s/download", chunkStorage.GetBaseURL()),
		"token":        token,
		"file_id":      f.Hash,
		"filename":     f.Name,
	})
}

// @Summary 上传完成通知
// @Description 直接上传完成后的回调通知
// @Tags 文件模块
// @Accept json
// @Produce json
// @Param file_id body string true "文件ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /files/upload-complete [post]
func UploadCompleteHandler(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID := c.MustGet("user_id").(uint)

	var req struct {
		FileID string `json:"file_id"`
		Hash   string `json:"hash"`
		Size   int64  `json:"size"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 从Redis获取上传信息
	rdb := c.MustGet("redis").(*redis.Client)
	ctx := context.Background()
	infoJson, err := rdb.Get(ctx, "pending_upload:"+req.FileID).Result()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "上传信息不存在或已过期"})
		return
	}

	// 解析上传信息
	var uploadInfo map[string]interface{}
	if err := json.Unmarshal([]byte(infoJson), &uploadInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析上传信息失败"})
		return
	}

	// 验证用户ID
	infoUserID := uint(uploadInfo["user_id"].(float64))
	if infoUserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权限完成此上传"})
		return
	}

	// 检查文件内容是否已存在
	var fileContent file.FileContent
	err = db.First(&fileContent, "hash = ?", req.Hash).Error
	if err == gorm.ErrRecordNotFound {
		fileContent = file.FileContent{
			Hash: req.Hash,
			Size: req.Size,
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

	// 创建文件记录
	f := file.File{
		Name:       uploadInfo["filename"].(string),
		Hash:       req.Hash,
		Type:       "file",
		ParentID:   uploadInfo["parent_id"].(string),
		OwnerID:    userID,
		UploadTime: time.Now(),
	}
	if err := db.Create(&f).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库写入失败", "detail": err.Error()})
		return
	}

	// 更新用户存储空间
	if err := user.UpdateUserStorageUsed(db, userID, fileContent.Size); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新存储空间失败", "detail": err.Error()})
		return
	}

	// 清理Redis缓存
	rdb.Del(ctx, "pending_upload:"+req.FileID)
	cacheKey := fmt.Sprintf("user:info:%d", userID)
	rdb.Del(ctx, cacheKey)
	fileListPrefix := fmt.Sprintf("filelist:%d:", userID)
	keys, _ := rdb.Keys(ctx, fileListPrefix+"*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "上传完成",
		"file_id": f.ID,
		"name":    f.Name,
		"size":    fileContent.Size,
	})
}

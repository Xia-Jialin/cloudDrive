package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"cloudDrive/internal/file"
	"cloudDrive/internal/user"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	if err := db.AutoMigrate(&file.File{}, &file.FileContent{}, &file.UserRoot{}, &user.User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func setupTestRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // 使用测试数据库
	})
}

func TestMultipartInitHandler_InstantUpload_ShouldCreateFileRecord(t *testing.T) {
	// 设置测试环境
	db := setupTestDB(t)
	rdb := setupTestRedis()
	defer rdb.FlushDB(rdb.Context())

	// 创建测试用户
	testUser := &user.User{
		ID:           1,
		Username:     "testuser",
		StorageLimit: 1024 * 1024 * 1024, // 1GB
		StorageUsed:  0,
	}
	db.Create(testUser)

	// 创建用户根目录
	userRoot := &file.UserRoot{
		UserID:    testUser.ID,
		RootID:    "root-id-123",
		CreatedAt: time.Now(),
	}
	db.Create(userRoot)

	// 预先创建一个文件内容记录（模拟已存在的文件）
	existingHash := "existing-file-hash-123"
	fileContent := &file.FileContent{
		Hash: existingHash,
		Size: 1024,
	}
	db.Create(fileContent)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", rdb)
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/files/multipart/init", MultipartInitHandler)

	// 准备请求数据
	reqData := map[string]interface{}{
		"name":        "test-file.txt",
		"size":        1024,
		"hash":        existingHash, // 使用已存在的hash，应该触发秒传
		"total_parts": 1,
		"parent_id":   "", // 使用根目录
	}
	jsonData, _ := json.Marshal(reqData)

	// 发送请求
	req, _ := http.NewRequest("POST", "/files/multipart/init", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["instant"].(bool))
	assert.Equal(t, "秒传成功", response["message"])
	assert.NotEmpty(t, response["file_id"])

	// 关键测试：验证是否创建了File记录
	var fileRecord file.File
	err = db.Where("hash = ? AND owner_id = ? AND name = ?", existingHash, testUser.ID, "test-file.txt").First(&fileRecord).Error
	assert.NoError(t, err, "File record should be created during instant upload")
	assert.Equal(t, "test-file.txt", fileRecord.Name)
	assert.Equal(t, existingHash, fileRecord.Hash)
	assert.Equal(t, testUser.ID, fileRecord.OwnerID)
	assert.Equal(t, "file", fileRecord.Type)
	assert.Equal(t, userRoot.RootID, fileRecord.ParentID)

	// 验证用户存储空间是否正确更新
	var updatedUser user.User
	db.First(&updatedUser, testUser.ID)
	assert.Equal(t, int64(1024), updatedUser.StorageUsed, "Storage should be updated during instant upload")
}

func TestMultipartInitHandler_InstantUpload_DuplicateName_ShouldFail(t *testing.T) {
	// 设置测试环境
	db := setupTestDB(t)
	rdb := setupTestRedis()
	defer rdb.FlushDB(rdb.Context())

	// 创建测试用户
	testUser := &user.User{
		ID:           1,
		Username:     "testuser",
		StorageLimit: 1024 * 1024 * 1024, // 1GB
		StorageUsed:  0,
	}
	db.Create(testUser)

	// 创建用户根目录
	userRoot := &file.UserRoot{
		UserID:    testUser.ID,
		RootID:    "root-id-123",
		CreatedAt: time.Now(),
	}
	db.Create(userRoot)

	// 预先创建一个文件内容记录
	existingHash := "existing-file-hash-123"
	fileContent := &file.FileContent{
		Hash: existingHash,
		Size: 1024,
	}
	db.Create(fileContent)

	// 预先创建一个同名文件
	existingFile := &file.File{
		Name:       "test-file.txt",
		Hash:       "different-hash",
		Type:       "file",
		ParentID:   userRoot.RootID,
		OwnerID:    testUser.ID,
		UploadTime: time.Now(),
	}
	db.Create(existingFile)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", rdb)
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/files/multipart/init", MultipartInitHandler)

	// 准备请求数据（尝试上传同名文件）
	reqData := map[string]interface{}{
		"name":        "test-file.txt", // 与已存在文件同名
		"size":        1024,
		"hash":        existingHash,
		"total_parts": 1,
		"parent_id":   "",
	}
	jsonData, _ := json.Marshal(reqData)

	// 发送请求
	req, _ := http.NewRequest("POST", "/files/multipart/init", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应 - 应该返回冲突错误
	assert.Equal(t, http.StatusConflict, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "同目录下已存在同名文件", response["error"])
}

func TestInstantUpload_IntegrationTest(t *testing.T) {
	// 设置测试环境
	db := setupTestDB(t)
	rdb := setupTestRedis()
	defer rdb.FlushDB(rdb.Context())

	// 创建测试用户
	testUser := &user.User{
		ID:           1,
		Username:     "testuser",
		StorageLimit: 1024 * 1024 * 1024, // 1GB
		StorageUsed:  0,
	}
	db.Create(testUser)

	// 创建用户根目录
	userRoot := &file.UserRoot{
		UserID:    testUser.ID,
		RootID:    "root-id-123",
		CreatedAt: time.Now(),
	}
	db.Create(userRoot)

	// 模拟第一次上传：创建文件内容记录
	fileHash := "test-file-hash-456"
	fileContent := &file.FileContent{
		Hash: fileHash,
		Size: 2048,
	}
	db.Create(fileContent)

	// 模拟第一次上传：创建文件记录
	firstFile := &file.File{
		Name:       "original-file.txt",
		Hash:       fileHash,
		Type:       "file",
		ParentID:   userRoot.RootID,
		OwnerID:    testUser.ID,
		UploadTime: time.Now(),
	}
	db.Create(firstFile)

	// 更新用户存储空间（模拟第一次上传后的状态）
	err := user.UpdateUserStorageUsed(db, testUser.ID, fileContent.Size)
	assert.NoError(t, err)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", rdb)
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/files/multipart/init", MultipartInitHandler)

	// 现在测试第二次上传相同内容的文件（应该触发秒传）
	reqData := map[string]interface{}{
		"name":        "duplicate-file.txt", // 不同的文件名，但相同的内容
		"size":        2048,
		"hash":        fileHash, // 相同的hash，应该触发秒传
		"total_parts": 1,
		"parent_id":   "",
	}
	jsonData, _ := json.Marshal(reqData)

	// 发送请求
	req, _ := http.NewRequest("POST", "/files/multipart/init", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["instant"].(bool))
	assert.Equal(t, "秒传成功", response["message"])

	// 验证数据库状态
	// 1. 应该有两个文件记录，但只有一个文件内容记录
	var fileCount int64
	db.Model(&file.File{}).Where("owner_id = ?", testUser.ID).Count(&fileCount)
	assert.Equal(t, int64(2), fileCount, "Should have 2 file records")

	var contentCount int64
	db.Model(&file.FileContent{}).Where("hash = ?", fileHash).Count(&contentCount)
	assert.Equal(t, int64(1), contentCount, "Should have only 1 file content record")

	// 2. 验证用户存储空间应该增加了第二个文件的大小
	var updatedUser user.User
	db.First(&updatedUser, testUser.ID)
	expectedStorageUsed := fileContent.Size * 2 // 两个文件，但实际存储只有一份
	assert.Equal(t, expectedStorageUsed, updatedUser.StorageUsed, "Storage should be updated for both files")

	// 3. 验证第二个文件记录的详细信息
	var secondFile file.File
	err = db.Where("name = ? AND owner_id = ?", "duplicate-file.txt", testUser.ID).First(&secondFile).Error
	assert.NoError(t, err)
	assert.Equal(t, fileHash, secondFile.Hash)
	assert.Equal(t, "file", secondFile.Type)
	assert.Equal(t, userRoot.RootID, secondFile.ParentID)

	t.Logf("Integration test passed: Instant upload correctly created file record and updated storage")
}

func TestInstantUpload_CacheCleaning(t *testing.T) {
	// 设置测试环境
	db := setupTestDB(t)
	rdb := setupTestRedis()

	// 检查Redis连接
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("Redis不可用，跳过缓存清理测试")
	}

	defer rdb.FlushDB(rdb.Context())

	// 创建测试用户
	testUser := &user.User{
		ID:           1,
		Username:     "testuser",
		StorageLimit: 1024 * 1024 * 1024, // 1GB
		StorageUsed:  0,
	}
	db.Create(testUser)

	// 创建用户根目录
	userRoot := &file.UserRoot{
		UserID:    testUser.ID,
		RootID:    "root-id-123",
		CreatedAt: time.Now(),
	}
	db.Create(userRoot)

	// 预先创建一个文件内容记录（模拟已存在的文件）
	existingHash := "existing-file-hash-123"
	fileContent := &file.FileContent{
		Hash: existingHash,
		Size: 1024,
	}
	db.Create(fileContent)

	// 预先在缓存中设置一些数据
	userCacheKey := fmt.Sprintf("user:info:%d", testUser.ID)
	fileListCacheKey := fmt.Sprintf("filelist:%d:root-id-123", testUser.ID)

	// 设置缓存数据
	rdb.Set(ctx, userCacheKey, `{"id":1,"username":"testuser","storage_used":0}`, time.Hour)
	rdb.Set(ctx, fileListCacheKey, `[{"id":"1","name":"old_file.txt"}]`, time.Hour)

	// 验证缓存数据已存在
	val, err := rdb.Get(ctx, userCacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)

	val, err = rdb.Get(ctx, fileListCacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", rdb)
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/files/multipart/init", MultipartInitHandler)

	// 构造请求
	reqBody := map[string]interface{}{
		"name":        "test-file.txt",
		"hash":        existingHash, // 使用已存在的hash触发秒传
		"size":        1024,
		"total_parts": 1,
		"parent_id":   "",
	}
	jsonBytes, _ := json.Marshal(reqBody)

	// 发送请求
	req, _ := http.NewRequest("POST", "/files/multipart/init", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.True(t, response["instant"].(bool))
	assert.Equal(t, "秒传成功", response["message"])

	// 验证缓存已被清理
	_, err = rdb.Get(ctx, userCacheKey).Result()
	assert.Equal(t, redis.Nil, err) // 缓存应该被删除

	// 验证文件列表缓存也被清理
	keys, err := rdb.Keys(ctx, fmt.Sprintf("filelist:%d:*", testUser.ID)).Result()
	assert.NoError(t, err)
	assert.Empty(t, keys) // 所有文件列表缓存都应该被删除

	// 验证数据库中确实创建了文件记录
	var createdFile file.File
	err = db.First(&createdFile, "name = ? AND owner_id = ?", "test-file.txt", testUser.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, existingHash, createdFile.Hash)

	// 验证用户存储空间被更新
	var updatedUser user.User
	err = db.First(&updatedUser, "id = ?", testUser.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, int64(1024), updatedUser.StorageUsed)

	t.Logf("缓存清理测试通过：秒传成功后正确清理了用户信息缓存和文件列表缓存")
}

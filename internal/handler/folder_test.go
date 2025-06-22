package handler

import (
	"bytes"
	"cloudDrive/internal/file"
	"cloudDrive/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockRedisClient 模拟Redis客户端
type MockRedisClient struct {
	data map[string]string
	mu   sync.RWMutex
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]string),
	}
}

func (m *MockRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("PONG")
	return cmd
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = fmt.Sprintf("%v", value)
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

func (m *MockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cmd := redis.NewStringCmd(ctx)
	if val, ok := m.data[key]; ok {
		cmd.SetVal(val)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (m *MockRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	deleted := int64(0)
	for _, key := range keys {
		if _, ok := m.data[key]; ok {
			delete(m.data, key)
			deleted++
		}
	}
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(deleted)
	return cmd
}

func (m *MockRedisClient) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []string
	// 简单的模式匹配，只支持 * 通配符
	for key := range m.data {
		if matchPattern(key, pattern) {
			keys = append(keys, key)
		}
	}
	cmd := redis.NewStringSliceCmd(ctx)
	cmd.SetVal(keys)
	return cmd
}

func (m *MockRedisClient) FlushDB(ctx context.Context) *redis.StatusCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string]string)
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

// 简单的模式匹配函数
func matchPattern(text, pattern string) bool {
	if pattern == "*" {
		return true
	}
	// 简单实现：如果模式以*结尾，检查前缀
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(text) >= len(prefix) && text[:len(prefix)] == prefix
	}
	return text == pattern
}

func setupTestDBForFolder(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	// 自动迁移
	db.AutoMigrate(&user.User{}, &file.File{}, &file.FileContent{}, &file.UserRoot{})
	return db
}

func TestCreateFolder_CacheCleaning(t *testing.T) {
	// 设置测试环境
	db := setupTestDBForFolder(t)
	rdb := NewMockRedisClient()

	ctx := context.Background()
	defer rdb.FlushDB(ctx)

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

	// 创建根目录文件夹
	rootFolder := &file.File{
		ID:         userRoot.RootID,
		Name:       "根目录",
		Type:       "folder",
		ParentID:   "",
		OwnerID:    testUser.ID,
		UploadTime: time.Now(),
	}
	db.Create(rootFolder)

	// 预先在缓存中设置一些数据
	fileListCacheKey1 := fmt.Sprintf("filelist:%d:%s:1:10:upload_time:desc", testUser.ID, userRoot.RootID)
	fileListCacheKey2 := fmt.Sprintf("filelist:%d::1:10:upload_time:desc", testUser.ID)

	// 设置缓存数据
	rdb.Set(ctx, fileListCacheKey1, `{"files":[{"id":"1","name":"old_file.txt"}],"total":1}`, time.Hour)
	rdb.Set(ctx, fileListCacheKey2, `{"files":[{"id":"1","name":"old_file.txt"}],"total":1}`, time.Hour)

	// 验证缓存数据已存在
	val, err := rdb.Get(ctx, fileListCacheKey1).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)

	val, err = rdb.Get(ctx, fileListCacheKey2).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", RedisInterface(rdb)) // 转换为接口类型
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/folders", CreateFolderHandler)

	// 构造请求
	reqBody := CreateFolderRequest{
		Name:     "新建文件夹",
		ParentID: userRoot.RootID,
	}
	jsonBytes, _ := json.Marshal(reqBody)

	// 发送请求
	req, _ := http.NewRequest("POST", "/folders", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "新建文件夹", response["name"])

	// 验证缓存已被清理
	keys, err := rdb.Keys(ctx, fmt.Sprintf("filelist:%d:*", testUser.ID)).Result()
	assert.NoError(t, err)
	assert.Empty(t, keys, "创建文件夹后应该清理所有文件列表缓存")

	// 验证数据库中确实创建了文件夹记录
	var createdFolder file.File
	err = db.First(&createdFolder, "name = ? AND owner_id = ? AND type = ?", "新建文件夹", testUser.ID, "folder").Error
	assert.NoError(t, err)
	assert.Equal(t, userRoot.RootID, createdFolder.ParentID)
	assert.Equal(t, "folder", createdFolder.Type)

	t.Logf("缓存清理测试通过：创建文件夹成功后正确清理了文件列表缓存")
}

func TestCreateFolder_NoCacheCleaningWhenError(t *testing.T) {
	// 设置测试环境
	db := setupTestDBForFolder(t)
	rdb := NewMockRedisClient()

	ctx := context.Background()
	defer rdb.FlushDB(ctx)

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

	// 创建根目录文件夹
	rootFolder := &file.File{
		ID:         userRoot.RootID,
		Name:       "根目录",
		Type:       "folder",
		ParentID:   "",
		OwnerID:    testUser.ID,
		UploadTime: time.Now(),
	}
	db.Create(rootFolder)

	// 先创建一个同名文件夹
	existingFolder := &file.File{
		ID:         "existing-folder-id",
		Name:       "重复文件夹",
		Type:       "folder",
		ParentID:   userRoot.RootID,
		OwnerID:    testUser.ID,
		UploadTime: time.Now(),
	}
	db.Create(existingFolder)

	// 预先在缓存中设置一些数据
	fileListCacheKey := fmt.Sprintf("filelist:%d:%s:1:10:upload_time:desc", testUser.ID, userRoot.RootID)
	rdb.Set(ctx, fileListCacheKey, `{"files":[{"id":"1","name":"old_file.txt"}],"total":1}`, time.Hour)

	// 验证缓存数据已存在
	val, err := rdb.Get(ctx, fileListCacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)

	// 设置Gin测试模式
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 设置中间件
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", RedisInterface(rdb)) // 转换为接口类型
		c.Set("user_id", testUser.ID)
		c.Next()
	})

	// 注册路由
	router.POST("/folders", CreateFolderHandler)

	// 构造请求（尝试创建同名文件夹）
	reqBody := CreateFolderRequest{
		Name:     "重复文件夹",
		ParentID: userRoot.RootID,
	}
	jsonBytes, _ := json.Marshal(reqBody)

	// 发送请求
	req, _ := http.NewRequest("POST", "/folders", bytes.NewBuffer(jsonBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 验证响应是错误
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)
	assert.Equal(t, "同目录下已存在同名文件夹", response["error"])

	// 验证缓存没有被清理（因为操作失败了）
	val, err = rdb.Get(ctx, fileListCacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val, "创建文件夹失败时不应该清理缓存")

	t.Logf("错误情况下缓存保持测试通过：创建文件夹失败时正确保持缓存不变")
}

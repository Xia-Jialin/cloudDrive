package handler

import (
	"cloudDrive/internal/file"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestRouter() (*gin.Engine, *gorm.DB, *redis.Client) {
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&file.File{}, &file.Share{})
	r := gin.Default()
	// 注入db和redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", rdb)
		c.Next()
	})
	r.GET("/api/share/:token", AccessPublicShareHandler)
	return r, db, rdb
}

func TestAccessPublicShare_ValidAndExpired(t *testing.T) {
	r, db, rdb := setupTestRouter()
	defer rdb.FlushDB(context.Background())
	// 创建测试文件
	fileID := "file-123"
	f := file.File{
		ID:      fileID,
		Name:    "test.txt",
		Type:    "file",
		OwnerID: 1,
	}
	db.Create(&f)
	// 创建未过期分享
	shareValid := file.Share{
		ResourceID: fileID,
		ShareType:  "public",
		Token:      "valid-token",
		ExpireAt:   time.Now().Add(1 * time.Hour),
		CreatorID:  1,
		CreatedAt:  time.Now(),
	}
	db.Create(&shareValid)
	// 创建已过期分享
	shareExpired := file.Share{
		ResourceID: fileID,
		ShareType:  "public",
		Token:      "expired-token",
		ExpireAt:   time.Now().Add(-1 * time.Hour),
		CreatorID:  1,
		CreatedAt:  time.Now(),
	}
	db.Create(&shareExpired)

	// 测试未过期分享
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/share/valid-token", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, fileID, resp["resource_id"])
	assert.Equal(t, "test.txt", resp["name"])

	// 测试已过期分享
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/share/expired-token", nil)
	r.ServeHTTP(w2, req2)
	assert.Equal(t, 410, w2.Code)
	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.Contains(t, resp2["error"], "已过期")
}

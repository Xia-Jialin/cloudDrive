package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestFileListCache(t *testing.T) {
	// 初始化Redis客户端（可用mock或本地测试实例）
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	ctx := context.Background()
	defer rdb.FlushDB(ctx)

	// 构造缓存key
	userID := 1
	parentID := "root"
	page := 1
	pageSize := 10
	orderBy := "upload_time"
	order := "desc"
	cacheKey := getFileListCacheKey(userID, parentID, page, pageSize, orderBy, order)

	// 1. 缓存未命中，返回空
	val, err := rdb.Get(ctx, cacheKey).Result()
	assert.Error(t, err)
	assert.Empty(t, val)

	// 2. 写入缓存，再次获取应命中
	mockResp := `{"files":[],"total":0}`
	err = rdb.Set(ctx, cacheKey, mockResp, 0).Err()
	assert.NoError(t, err)
	val, err = rdb.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, mockResp, val)

	// 3. 删除缓存
	rdb.Del(ctx, cacheKey)
	val, err = rdb.Get(ctx, cacheKey).Result()
	assert.Error(t, err)
}

// getFileListCacheKey 生成文件列表缓存key
func getFileListCacheKey(userID int, parentID string, page, pageSize int, orderBy, order string) string {
	return "filelist:" +
		fmt.Sprintf("%d:%s:%d:%d:%s:%s", userID, parentID, page, pageSize, orderBy, order)
}

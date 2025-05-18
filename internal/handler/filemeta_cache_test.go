package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestFileMetaCache(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	ctx := context.Background()
	defer rdb.FlushDB(ctx)

	fileID := "test-file-id"
	cacheKey := getFileMetaCacheKey(fileID)

	// 1. 缓存未命中，返回空
	val, err := rdb.Get(ctx, cacheKey).Result()
	assert.Error(t, err)
	assert.Empty(t, val)

	// 2. 写入缓存，再次获取应命中
	mockResp := `{"id":"test-file-id","name":"test.txt","size":123}`
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

// getFileMetaCacheKey 生成文件元数据缓存key
func getFileMetaCacheKey(fileID string) string {
	return fmt.Sprintf("filemeta:%s", fileID)
}

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

// mockShareData 用于模拟分享信息结构
var mockShareData = map[string]interface{}{
	"resource_id": "file-123",
	"name":        "test.txt",
	"type":        "file",
	"owner_id":    1,
	"expire_at":   time.Now().Add(1 * time.Hour).Unix(),
}

func getShareCacheKey(token string) string {
	return fmt.Sprintf("share:%s", token)
}

func TestShareCache_Hit(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	ctx := context.Background()
	defer rdb.FlushDB(ctx)

	token := "token-hit"
	cacheKey := getShareCacheKey(token)
	data, _ := json.Marshal(mockShareData)
	err := rdb.Set(ctx, cacheKey, data, 5*time.Minute).Err()
	assert.NoError(t, err)

	val, err := rdb.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	var got map[string]interface{}
	json.Unmarshal([]byte(val), &got)
	assert.Equal(t, mockShareData["resource_id"], got["resource_id"])
}

func TestShareCache_MissAndWrite(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	ctx := context.Background()
	defer rdb.FlushDB(ctx)

	token := "token-miss"
	cacheKey := getShareCacheKey(token)
	// 1. Redis未命中
	val, err := rdb.Get(ctx, cacheKey).Result()
	assert.Error(t, err)
	assert.Empty(t, val)
	// 2. 写入缓存
	data, _ := json.Marshal(mockShareData)
	err = rdb.Set(ctx, cacheKey, data, 5*time.Minute).Err()
	assert.NoError(t, err)
	val, err = rdb.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, val)
}

func TestShareCache_Delete(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
	ctx := context.Background()
	defer rdb.FlushDB(ctx)

	token := "token-del"
	cacheKey := getShareCacheKey(token)
	data, _ := json.Marshal(mockShareData)
	rdb.Set(ctx, cacheKey, data, 5*time.Minute)
	// 删除缓存
	rdb.Del(ctx, cacheKey)
	val, err := rdb.Get(ctx, cacheKey).Result()
	assert.Error(t, err)
	assert.Empty(t, val)
}

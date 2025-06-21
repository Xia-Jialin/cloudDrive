package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"cloudDrive/internal/storage"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v4"
)

// StorageService 存储服务接口
type StorageService interface {
	// 基础存储操作
	Save(ctx context.Context, key string, content io.Reader) error
	Read(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error

	// 分片上传操作
	InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error)
	UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader) (string, error)
	CompleteMultipartUpload(ctx context.Context, uploadID string, parts []storage.PartInfo) (string, error)
	ListParts(ctx context.Context, uploadID string) ([]int, error)

	// 验证令牌
	VerifyToken(ctx context.Context, token string, operation string) (map[string]interface{}, error)

	// 计算文件哈希
	CalculateFileHash(filePath string) (string, error)
}

// StorageServiceImpl 存储服务实现
type StorageServiceImpl struct {
	storage storage.Storage
	redis   *redis.Client
}

// NewStorageService 创建存储服务实例
func NewStorageService(storage storage.Storage, redis *redis.Client) *StorageServiceImpl {
	return &StorageServiceImpl{
		storage: storage,
		redis:   redis,
	}
}

// Save 保存文件
func (s *StorageServiceImpl) Save(ctx context.Context, key string, content io.Reader) error {
	return s.storage.Upload(ctx, key, content)
}

// Read 读取文件
func (s *StorageServiceImpl) Read(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// Delete 删除文件
func (s *StorageServiceImpl) Delete(ctx context.Context, key string) error {
	return s.storage.Delete(ctx, key)
}

// InitMultipartUpload 初始化分片上传
func (s *StorageServiceImpl) InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error) {
	return s.storage.InitMultipartUpload(ctx, fileID, filename)
}

// UploadPart 上传分片
func (s *StorageServiceImpl) UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader) (string, error) {
	return s.storage.UploadPart(ctx, uploadID, partNumber, partData)
}

// CompleteMultipartUpload 完成分片上传
func (s *StorageServiceImpl) CompleteMultipartUpload(ctx context.Context, uploadID string, parts []storage.PartInfo) (string, error) {
	return s.storage.CompleteMultipartUpload(ctx, uploadID, parts)
}

// ListParts 列出已上传的分片
func (s *StorageServiceImpl) ListParts(ctx context.Context, uploadID string) ([]int, error) {
	// 检查存储接口是否实现了ListUploadedParts方法
	if chunkStorage, ok := s.storage.(*storage.ChunkServerStorage); ok {
		return chunkStorage.ListUploadedParts(ctx, uploadID)
	}

	// 对于其他存储类型，可以从redis或其他地方获取分片信息
	key := fmt.Sprintf("chunk:multipart:%s:parts", uploadID)
	result, err := s.redis.SMembers(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return []int{}, nil
		}
		return nil, fmt.Errorf("获取分片列表失败: %v", err)
	}

	// 将字符串转换为整数
	parts := make([]int, 0, len(result))
	for _, partStr := range result {
		partNum, err := strconv.Atoi(partStr)
		if err != nil {
			continue
		}
		parts = append(parts, partNum)
	}

	return parts, nil
}

// VerifyToken 验证令牌
func (s *StorageServiceImpl) VerifyToken(ctx context.Context, token string, operation string) (map[string]interface{}, error) {
	// 从Redis获取令牌信息
	key := fmt.Sprintf("chunk:token:%s", token)
	val, err := s.redis.Get(ctx, key).Result()
	if err == nil {
		// 对于新的令牌格式，值就是fileID
		tokenInfo := map[string]interface{}{
			"file_id": val,
		}
		return tokenInfo, nil
	}

	// 如果Redis中没有找到令牌，尝试使用JWT验证
	if err == redis.Nil {
		// 尝试解析JWT令牌
		tokenClaims, err := parseJWTToken(token)
		if err != nil {
			return nil, errors.New("令牌无效或已过期")
		}

		// 检查令牌是否过期
		if exp, ok := tokenClaims["exp"].(float64); ok {
			if time.Now().Unix() > int64(exp) {
				return nil, errors.New("令牌已过期")
			}
		}

		// 从JWT中获取fileID
		fileID, ok := tokenClaims["file_id"].(string)
		if !ok || fileID == "" {
			return nil, errors.New("无效的文件ID")
		}

		// 将令牌信息保存到Redis，有效期1小时
		s.redis.Set(ctx, key, fileID, time.Hour)

		return tokenClaims, nil
	}

	return nil, fmt.Errorf("验证令牌失败: %v", err)
}

// parseJWTToken 解析JWT令牌
func parseJWTToken(tokenString string) (map[string]interface{}, error) {
	// 使用jwt库直接解析
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 这里应该使用与签名时相同的密钥
		return []byte("your-super-secret-key-for-jwt-token-signing"), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// 将claims转换为map[string]interface{}
		result := make(map[string]interface{})
		for key, val := range claims {
			result[key] = val
		}
		return result, nil
	} else {
		return nil, errors.New("无效的令牌")
	}
}

// CalculateFileHash 计算文件的SHA256哈希值
func (s *StorageServiceImpl) CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	// 使用SHA256算法
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("计算哈希失败: %v", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GetRedisClient 获取Redis客户端
func (s *StorageServiceImpl) GetRedisClient() *redis.Client {
	return s.redis
}

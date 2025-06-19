package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"cloudDrive/internal/storage"

	"github.com/go-redis/redis/v8"
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

// VerifyToken 验证令牌
func (s *StorageServiceImpl) VerifyToken(ctx context.Context, token string, operation string) (map[string]interface{}, error) {
	// 从Redis获取令牌信息
	key := fmt.Sprintf("chunk:token:%s", token)
	val, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, errors.New("令牌无效或已过期")
		}
		return nil, fmt.Errorf("验证令牌失败: %v", err)
	}

	// 对于新的令牌格式，值就是fileID
	tokenInfo := map[string]interface{}{
		"file_id": val,
	}

	return tokenInfo, nil
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

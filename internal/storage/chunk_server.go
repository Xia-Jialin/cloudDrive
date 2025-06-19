package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-redis/redis/v8"
)

// ChunkServerStorage 是块存储服务的适配器
type ChunkServerStorage struct {
	Client      *ChunkClient
	RedisClient *redis.Client
	TempDir     string
}

// NewChunkServerStorage 创建一个新的块存储服务适配器
func NewChunkServerStorage(baseURL string, redisClient *redis.Client, tempDir string) (*ChunkServerStorage, error) {
	// 确保临时目录存在
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %v", err)
	}

	return &ChunkServerStorage{
		Client:      NewChunkClient(baseURL),
		RedisClient: redisClient,
		TempDir:     tempDir,
	}, nil
}

// 生成上传令牌
func (s *ChunkServerStorage) generateToken(ctx context.Context, operation string, fileID string) (string, error) {
	token := fmt.Sprintf("%s:%s:%d", operation, fileID, time.Now().UnixNano())

	// 将令牌存储在Redis中，有效期5分钟
	err := s.RedisClient.Set(ctx, "chunk:token:"+token, fileID, 5*time.Minute).Err()
	if err != nil {
		return "", fmt.Errorf("生成令牌失败: %v", err)
	}

	return token, nil
}

// Upload 实现Storage接口的Upload方法
func (s *ChunkServerStorage) Upload(ctx context.Context, fileID string, reader io.Reader) error {
	// 创建临时文件
	tempFile := filepath.Join(s.TempDir, fileID)
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tempFile) // 上传完成后删除临时文件

	// 将数据写入临时文件
	if _, err := io.Copy(file, reader); err != nil {
		file.Close()
		return fmt.Errorf("写入临时文件失败: %v", err)
	}
	file.Close()

	// 生成上传令牌
	token, err := s.generateToken(ctx, "upload", fileID)
	if err != nil {
		return err
	}

	// 通过块存储服务上传文件
	_, err = s.Client.UploadFile(tempFile, token)
	if err != nil {
		return fmt.Errorf("通过块存储服务上传文件失败: %v", err)
	}

	return nil
}

// Download 实现Storage接口的Download方法
func (s *ChunkServerStorage) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	// 创建临时文件路径
	tempFile := filepath.Join(s.TempDir, fileID+"_download")

	// 生成下载令牌
	token, err := s.generateToken(ctx, "download", fileID)
	if err != nil {
		return nil, err
	}

	// 通过块存储服务下载文件
	if err := s.Client.DownloadFile(fileID, token, tempFile); err != nil {
		return nil, fmt.Errorf("通过块存储服务下载文件失败: %v", err)
	}

	// 打开下载的文件
	file, err := os.Open(tempFile)
	if err != nil {
		return nil, fmt.Errorf("打开下载的文件失败: %v", err)
	}

	// 创建一个自定义的ReadCloser，在关闭时删除临时文件
	return &customReadCloser{
		ReadCloser: file,
		onClose: func() error {
			return os.Remove(tempFile)
		},
	}, nil
}

// Delete 实现Storage接口的Delete方法
func (s *ChunkServerStorage) Delete(ctx context.Context, fileID string) error {
	// 生成删除令牌
	token, err := s.generateToken(ctx, "delete", fileID)
	if err != nil {
		return err
	}

	// 构造删除请求
	params := url.Values{}
	params.Add("file_id", fileID)
	params.Add("token", token)

	// 发送删除请求
	resp, err := s.Client.HTTPClient.PostForm(fmt.Sprintf("%s/delete", s.Client.BaseURL), params)
	if err != nil {
		return fmt.Errorf("发送删除请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("删除文件失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// InitMultipartUpload 初始化分片上传
func (s *ChunkServerStorage) InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error) {
	// 生成上传令牌
	token, err := s.generateToken(ctx, "multipart_init", fileID)
	if err != nil {
		return "", err
	}

	// 通过块存储服务初始化分片上传
	uploadID, err := s.Client.InitMultipartUpload(filename, token)
	if err != nil {
		return "", fmt.Errorf("初始化分片上传失败: %v", err)
	}

	// 将uploadID与fileID关联存储在Redis中
	err = s.RedisClient.Set(ctx, "chunk:upload:"+uploadID, fileID, 24*time.Hour).Err()
	if err != nil {
		return "", fmt.Errorf("存储uploadID关联失败: %v", err)
	}

	return uploadID, nil
}

// UploadPart 上传分片
func (s *ChunkServerStorage) UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader) (string, error) {
	// 检查uploadID是否有效
	fileID, err := s.RedisClient.Get(ctx, "chunk:upload:"+uploadID).Result()
	if err != nil {
		return "", fmt.Errorf("无效的uploadID: %v", err)
	}

	// 生成上传令牌
	token, err := s.generateToken(ctx, "multipart_upload", fileID)
	if err != nil {
		return "", err
	}

	// 读取分片数据
	data, err := io.ReadAll(partData)
	if err != nil {
		return "", fmt.Errorf("读取分片数据失败: %v", err)
	}

	// 通过块存储服务上传分片
	etag, err := s.Client.UploadPart(uploadID, partNumber, data, token)
	if err != nil {
		return "", fmt.Errorf("上传分片失败: %v", err)
	}

	return etag, nil
}

// CompleteMultipartUpload 完成分片上传
func (s *ChunkServerStorage) CompleteMultipartUpload(ctx context.Context, uploadID string, parts []PartInfo) (string, error) {
	// 检查uploadID是否有效
	fileID, err := s.RedisClient.Get(ctx, "chunk:upload:"+uploadID).Result()
	if err != nil {
		return "", fmt.Errorf("无效的uploadID: %v", err)
	}

	// 生成上传令牌
	token, err := s.generateToken(ctx, "multipart_complete", fileID)
	if err != nil {
		return "", err
	}

	// 通过块存储服务完成分片上传
	resultFileID, err := s.Client.CompleteMultipartUpload(uploadID, parts, token)
	if err != nil {
		return "", fmt.Errorf("完成分片上传失败: %v", err)
	}

	// 删除Redis中的uploadID关联
	s.RedisClient.Del(ctx, "chunk:upload:"+uploadID)

	return resultFileID, nil
}

// ListUploadedParts 列出已上传的分片
func (s *ChunkServerStorage) ListUploadedParts(ctx context.Context, uploadID string) ([]int, error) {
	// 检查uploadID是否有效
	fileID, err := s.RedisClient.Get(ctx, "chunk:upload:"+uploadID).Result()
	if err != nil {
		return nil, fmt.Errorf("无效的uploadID: %v", err)
	}

	// 生成令牌
	token, err := s.generateToken(ctx, "multipart_list", fileID)
	if err != nil {
		return nil, err
	}

	// 构造请求参数
	params := url.Values{}
	params.Add("upload_id", uploadID)
	params.Add("token", token)

	// 发送请求获取已上传分片列表
	resp, err := s.Client.HTTPClient.Get(fmt.Sprintf("%s/multipart/list?%s", s.Client.BaseURL, params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("获取已上传分片列表失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("获取已上传分片列表失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Parts []int `json:"parts"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取已上传分片列表失败: %s", result.Message)
	}

	return result.Data.Parts, nil
}

// customReadCloser 是一个自定义的ReadCloser，在关闭时执行额外的操作
type customReadCloser struct {
	io.ReadCloser
	onClose func() error
}

func (c *customReadCloser) Close() error {
	err1 := c.ReadCloser.Close()
	err2 := c.onClose()
	if err1 != nil {
		return err1
	}
	return err2
}

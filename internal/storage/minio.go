package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStorage struct {
	Client *minio.Client
	Bucket string
	TmpDir string // 临时目录，用于存储分片
}

func NewMinioStorage(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinioStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, err
		}
	}

	// 创建临时目录
	tmpDir := os.TempDir() + "/minio_multipart"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, err
	}

	return &MinioStorage{
		Client: client,
		Bucket: bucket,
		TmpDir: tmpDir,
	}, nil
}

// 实现新的Storage接口

// Upload 上传文件
func (m *MinioStorage) Upload(ctx context.Context, fileID string, reader io.Reader) error {
	_, err := m.Client.PutObject(ctx, m.Bucket, fileID, reader, -1, minio.PutObjectOptions{})
	return err
}

// Download 下载文件
func (m *MinioStorage) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	return m.Client.GetObject(ctx, m.Bucket, fileID, minio.GetObjectOptions{})
}

// Delete 删除文件
func (m *MinioStorage) Delete(ctx context.Context, fileID string) error {
	return m.Client.RemoveObject(ctx, m.Bucket, fileID, minio.RemoveObjectOptions{})
}

// InitMultipartUpload 初始化分片上传
func (m *MinioStorage) InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error) {
	// 创建一个唯一的上传ID，使用绝对值确保是正数
	timestamp := time.Now().UnixNano()
	if timestamp < 0 {
		timestamp = -timestamp // 确保是正数
	}
	uploadID := fmt.Sprintf("%s_%d", fileID, timestamp)

	// 创建上传ID对应的目录
	dir := filepath.Join(m.TmpDir, uploadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// 存储元数据
	metaPath := filepath.Join(dir, "meta")
	if err := os.WriteFile(metaPath, []byte(fileID), 0644); err != nil {
		return "", err
	}

	return uploadID, nil
}

// UploadPart 上传分片
func (m *MinioStorage) UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader, options ...interface{}) (string, error) {
	// 创建上传ID对应的目录
	dir := filepath.Join(m.TmpDir, uploadID)

	// 保存分片到临时文件
	partPath := filepath.Join(dir, fmt.Sprintf("%d", partNumber))
	out, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// 写入分片数据
	_, err = io.Copy(out, partData)
	if err != nil {
		return "", err
	}

	// 简单地使用文件路径作为ETag
	return partPath, nil
}

// CompleteMultipartUpload 完成分片上传
func (m *MinioStorage) CompleteMultipartUpload(ctx context.Context, uploadID string, parts []PartInfo) (string, error) {
	dir := filepath.Join(m.TmpDir, uploadID)

	// 读取元数据获取fileID
	metaPath := filepath.Join(dir, "meta")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return "", fmt.Errorf("读取上传元数据失败: %v", err)
	}
	fileID := string(metaData)

	// 创建临时合并文件
	mergedFilePath := filepath.Join(m.TmpDir, uploadID+".merged")
	out, err := os.Create(mergedFilePath)
	if err != nil {
		return "", err
	}
	defer func() {
		out.Close()
		os.Remove(mergedFilePath)
	}()

	// 按照part_number排序
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	// 合并所有分片
	for _, part := range parts {
		partPath := filepath.Join(dir, fmt.Sprintf("%d", part.PartNumber))
		in, err := os.Open(partPath)
		if err != nil {
			return "", fmt.Errorf("打开分片 %d 失败: %v", part.PartNumber, err)
		}

		if _, err := io.Copy(out, in); err != nil {
			in.Close()
			return "", err
		}
		in.Close()
	}

	// 重新打开合并后的文件
	out.Close()
	mergedFile, err := os.Open(mergedFilePath)
	if err != nil {
		return "", err
	}
	defer mergedFile.Close()

	// 上传到MinIO
	_, err = m.Client.PutObject(
		ctx,
		m.Bucket,
		fileID,
		mergedFile,
		-1,
		minio.PutObjectOptions{},
	)
	if err != nil {
		return "", err
	}

	// 清理临时目录
	os.RemoveAll(dir)

	return fileID, nil
}

// 兼容旧版接口

// Save 保存内容到MinIO
func (m *MinioStorage) Save(key string, content io.Reader) error {
	_, err := m.Client.PutObject(context.Background(), m.Bucket, key, content, -1, minio.PutObjectOptions{})
	return err
}

// Read 从MinIO读取内容
func (m *MinioStorage) Read(key string) ([]byte, error) {
	obj, err := m.Client.GetObject(context.Background(), m.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SavePart 保存分片
func (m *MinioStorage) SavePart(uploadId string, partNumber int, data []byte) error {
	// 创建上传ID对应的目录
	dir := filepath.Join(m.TmpDir, uploadId)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 保存分片到临时文件
	partPath := filepath.Join(dir, fmt.Sprintf("%d", partNumber))
	return os.WriteFile(partPath, data, 0644)
}

// MergeParts 合并所有分片为目标文件
func (m *MinioStorage) MergeParts(uploadId string, totalParts int, targetKey string) error {
	dir := filepath.Join(m.TmpDir, uploadId)

	// 创建临时合并文件
	mergedFilePath := filepath.Join(m.TmpDir, uploadId+".merged")
	out, err := os.Create(mergedFilePath)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		os.Remove(mergedFilePath)
	}()

	// 按顺序合并所有分片
	for i := 1; i <= totalParts; i++ {
		partPath := filepath.Join(dir, fmt.Sprintf("%d", i))
		in, err := os.Open(partPath)
		if err != nil {
			return fmt.Errorf("missing part %d: %w", i, err)
		}
		if _, err := io.Copy(out, in); err != nil {
			in.Close()
			return err
		}
		in.Close()
	}

	// 重新打开合并后的文件
	out.Close()
	mergedFile, err := os.Open(mergedFilePath)
	if err != nil {
		return err
	}
	defer mergedFile.Close()

	// 上传到MinIO
	_, err = m.Client.PutObject(
		context.Background(),
		m.Bucket,
		targetKey,
		mergedFile,
		-1,
		minio.PutObjectOptions{},
	)
	if err != nil {
		return err
	}

	// 清理临时目录
	return os.RemoveAll(dir)
}

// ListUploadedParts 查询已上传分片序号
func (m *MinioStorage) ListUploadedParts(ctx context.Context, uploadId string) ([]int, error) {
	dir := filepath.Join(m.TmpDir, uploadId)
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []int{}, nil // 目录不存在时返回空
	}
	if err != nil {
		return nil, err
	}

	var parts []int
	for _, f := range files {
		if !f.IsDir() {
			n, err := strconv.Atoi(f.Name())
			if err == nil {
				parts = append(parts, n)
			}
		}
	}
	sort.Ints(parts)
	return parts, nil
}

// RemoveUploadTemp 删除分片临时目录
func (m *MinioStorage) RemoveUploadTemp(uploadId string) error {
	dir := filepath.Join(m.TmpDir, uploadId)
	return os.RemoveAll(dir)
}

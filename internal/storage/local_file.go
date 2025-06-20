package storage

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// LocalFileStorage 实现 Storage 接口，基于本地文件系统
// key 直接作为文件名（可根据需要加前缀或目录）
type LocalFileStorage struct {
	Dir string // 存储根目录
}

// 实现新的Storage接口

// Upload 上传文件
func (l *LocalFileStorage) Upload(ctx context.Context, fileID string, reader io.Reader) error {
	return l.Save(fileID, reader)
}

// Download 下载文件
func (l *LocalFileStorage) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	filePath := filepath.Join(l.Dir, fileID)
	return os.Open(filePath)
}

// Delete 删除文件
func (l *LocalFileStorage) Delete(ctx context.Context, fileID string) error {
	filePath := filepath.Join(l.Dir, fileID)
	return os.Remove(filePath)
}

// InitMultipartUpload 初始化分片上传
func (l *LocalFileStorage) InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error) {
	uploadID := fmt.Sprintf("%s_%d", fileID, os.Getpid())
	dir := filepath.Join(l.Dir, "multipart", uploadID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	// 创建一个元数据文件，记录文件名
	metaPath := filepath.Join(dir, "meta")
	if err := ioutil.WriteFile(metaPath, []byte(filename), 0644); err != nil {
		return "", err
	}
	return uploadID, nil
}

// UploadPart 上传分片
func (l *LocalFileStorage) UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader, options ...interface{}) (string, error) {
	dir := filepath.Join(l.Dir, "multipart", uploadID)
	partPath := filepath.Join(dir, fmt.Sprintf("%d", partNumber))

	// 创建分片文件
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

	// 简单地返回分片路径作为ETag
	return partPath, nil
}

// CompleteMultipartUpload 完成分片上传
func (l *LocalFileStorage) CompleteMultipartUpload(ctx context.Context, uploadID string, parts []PartInfo) (string, error) {
	dir := filepath.Join(l.Dir, "multipart", uploadID)

	// 读取元数据文件获取原始文件名
	metaPath := filepath.Join(dir, "meta")
	metaData, err := ioutil.ReadFile(metaPath)
	if err != nil {
		return "", fmt.Errorf("读取上传元数据失败: %v", err)
	}

	// 使用fileID作为目标文件名
	fileID := string(metaData)
	targetPath := filepath.Join(l.Dir, fileID)

	// 创建目标文件
	out, err := os.Create(targetPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

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

	// 清理临时目录
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}

	return fileID, nil
}

// 兼容旧版接口

func (l *LocalFileStorage) Save(key string, content io.Reader) error {
	filePath := filepath.Join(l.Dir, key)
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, content)
	return err
}

func (l *LocalFileStorage) Read(key string) ([]byte, error) {
	filePath := filepath.Join(l.Dir, key)
	return ioutil.ReadFile(filePath)
}

// SavePart 保存分片
func (l *LocalFileStorage) SavePart(uploadId string, partNumber int, data []byte) error {
	dir := filepath.Join(l.Dir, "multipart", uploadId)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	partPath := filepath.Join(dir, fmt.Sprintf("%d", partNumber))
	return ioutil.WriteFile(partPath, data, 0644)
}

// MergeParts 合并所有分片为目标文件
func (l *LocalFileStorage) MergeParts(uploadId string, totalParts int, targetKey string) error {
	dir := filepath.Join(l.Dir, "multipart", uploadId)
	targetPath := filepath.Join(l.Dir, targetKey)
	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()
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
	return os.RemoveAll(dir)
}

// ListUploadedParts 查询已上传分片序号
func (l *LocalFileStorage) ListUploadedParts(ctx context.Context, uploadId string) ([]int, error) {
	dir := filepath.Join(l.Dir, "multipart", uploadId)
	files, err := ioutil.ReadDir(dir)
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
func (l *LocalFileStorage) RemoveUploadTemp(uploadId string) error {
	dir := filepath.Join(l.Dir, "multipart", uploadId)
	return os.RemoveAll(dir)
}

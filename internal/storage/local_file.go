package storage

import (
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

func (l *LocalFileStorage) Save(key string, content io.Reader) error {
	filePath := l.Dir + "/" + key
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, content)
	return err
}

func (l *LocalFileStorage) Read(key string) ([]byte, error) {
	filePath := l.Dir + "/" + key
	return ioutil.ReadFile(filePath)
}

func (l *LocalFileStorage) Delete(key string) error {
	filePath := l.Dir + "/" + key
	return os.Remove(filePath)
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
func (l *LocalFileStorage) ListUploadedParts(uploadId string) ([]int, error) {
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

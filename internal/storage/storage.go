package storage

import (
	"io"
)

// FileInfo 表示key/value结构
// Key 为唯一标识，Content 为内容
type FileInfo struct {
	Key     string
	Content []byte
}

// Storage 定义通用的key/value存储接口
// Save 保存内容到指定key
// Read 读取指定key的内容
// Delete 删除指定key的内容
type Storage interface {
	Save(key string, content io.Reader) error
	Read(key string) ([]byte, error)
	Delete(key string) error
}

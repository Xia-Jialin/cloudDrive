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

// MultipartStorage 定义分片上传相关接口
// uploadId: 本次分片上传唯一标识，partNumber: 分片序号，data: 分片内容
// totalParts: 总分片数，targetKey: 合并后目标文件key
type MultipartStorage interface {
	SavePart(uploadId string, partNumber int, data []byte) error
	MergeParts(uploadId string, totalParts int, targetKey string) error
	ListUploadedParts(uploadId string) ([]int, error)
	RemoveUploadTemp(uploadId string) error
}

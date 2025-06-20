package storage

import (
	"context"
	"io"
)

// FileInfo 表示key/value结构
// Key 为唯一标识，Content 为内容
type FileInfo struct {
	Key     string
	Content []byte
}

// PartInfo 表示分片信息
type PartInfo struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

// Storage 定义通用的存储接口
type Storage interface {
	// 基本文件操作
	Upload(ctx context.Context, fileID string, reader io.Reader) error
	Download(ctx context.Context, fileID string) (io.ReadCloser, error)
	Delete(ctx context.Context, fileID string) error

	// 分片上传相关方法
	InitMultipartUpload(ctx context.Context, fileID string, filename string) (string, error)
	UploadPart(ctx context.Context, uploadID string, partNumber int, partData io.Reader, options ...interface{}) (string, error)
	CompleteMultipartUpload(ctx context.Context, uploadID string, parts []PartInfo) (string, error)
	ListUploadedParts(ctx context.Context, uploadID string) ([]int, error)
}

// 兼容旧版接口的方法
type LegacyStorage interface {
	Save(key string, content io.Reader) error
	Read(key string) ([]byte, error)
	Delete(key string) error

	// 旧版分片上传相关方法
	SavePart(uploadId string, partNumber int, data []byte) error
	MergeParts(uploadId string, totalParts int, targetKey string) error
	ListUploadedParts(uploadId string) ([]int, error)
	RemoveUploadTemp(uploadId string) error
}

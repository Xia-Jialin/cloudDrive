package api

import (
	"bytes"
	"context"
	"fmt"
	"net"

	"cloudDrive/cmd/chunkserver/internal/service"
	"cloudDrive/internal/storage"

	"google.golang.org/grpc"
)

// 由于我们还没有生成protobuf代码，这里先定义一些临时的接口
// 在实际实现中，这些应该由protobuf生成

// StorageServiceServer 存储服务gRPC接口
type StorageServiceServer interface {
	Upload(context.Context, *UploadRequest) (*UploadResponse, error)
	Download(context.Context, *DownloadRequest) (*DownloadResponse, error)
	Delete(context.Context, *DeleteRequest) (*DeleteResponse, error)
	InitMultipartUpload(context.Context, *InitMultipartUploadRequest) (*InitMultipartUploadResponse, error)
	UploadPart(context.Context, *UploadPartRequest) (*UploadPartResponse, error)
	CompleteMultipartUpload(context.Context, *CompleteMultipartUploadRequest) (*CompleteMultipartUploadResponse, error)
}

// 临时请求/响应结构体，实际应由protobuf生成
type UploadRequest struct {
	FileID  string
	Content []byte
	Token   string
}

type UploadResponse struct {
	Success bool
	Error   string
}

type DownloadRequest struct {
	FileID string
	Token  string
}

type DownloadResponse struct {
	Content []byte
	Error   string
}

type DeleteRequest struct {
	FileID string
	Token  string
}

type DeleteResponse struct {
	Success bool
	Error   string
}

type InitMultipartUploadRequest struct {
	FileID   string
	Filename string
	Token    string
}

type InitMultipartUploadResponse struct {
	UploadID string
	Error    string
}

type UploadPartRequest struct {
	UploadID   string
	PartNumber int32
	Data       []byte
	Token      string
}

type UploadPartResponse struct {
	ETag  string
	Error string
}

type PartInfo struct {
	PartNumber int32
	ETag       string
}

type CompleteMultipartUploadRequest struct {
	UploadID string
	Parts    []PartInfo
	Token    string
}

type CompleteMultipartUploadResponse struct {
	FileID string
	Error  string
}

// GRPCServer gRPC服务器
type GRPCServer struct {
	service *service.StorageServiceImpl
	server  *grpc.Server
}

// NewGRPCServer 创建gRPC服务器
func NewGRPCServer(service *service.StorageServiceImpl) *GRPCServer {
	server := grpc.NewServer()
	s := &GRPCServer{
		service: service,
		server:  server,
	}

	// 在实际实现中，这里应该注册由protobuf生成的服务
	// 例如: pb.RegisterStorageServiceServer(server, s)

	return s
}

// Start 启动gRPC服务器
func (s *GRPCServer) Start(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	return s.server.Serve(lis)
}

// GracefulStop 优雅关闭gRPC服务器
func (s *GRPCServer) GracefulStop() {
	s.server.GracefulStop()
}

// Upload 实现Upload RPC方法
func (s *GRPCServer) Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
	// 验证令牌
	_, err := s.service.VerifyToken(ctx, req.Token, "upload")
	if err != nil {
		return &UploadResponse{Success: false, Error: err.Error()}, nil
	}

	// 上传文件
	err = s.service.Save(ctx, req.FileID, bytes.NewReader(req.Content))
	if err != nil {
		return &UploadResponse{Success: false, Error: err.Error()}, nil
	}
	return &UploadResponse{Success: true}, nil
}

// Download 实现Download RPC方法
func (s *GRPCServer) Download(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error) {
	// 验证令牌
	tokenInfo, err := s.service.VerifyToken(ctx, req.Token, "download")
	if err != nil {
		return &DownloadResponse{Error: err.Error()}, nil
	}

	// 验证文件ID是否匹配
	storedFileID, ok := tokenInfo["file_id"].(string)
	if !ok || storedFileID != req.FileID {
		return &DownloadResponse{Error: "令牌与请求文件不匹配"}, nil
	}

	// 读取文件内容
	data, err := s.service.Read(ctx, req.FileID)
	if err != nil {
		return &DownloadResponse{Error: err.Error()}, nil
	}
	return &DownloadResponse{Content: data}, nil
}

// Delete 实现Delete RPC方法
func (s *GRPCServer) Delete(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error) {
	// 验证令牌
	tokenInfo, err := s.service.VerifyToken(ctx, req.Token, "delete")
	if err != nil {
		return &DeleteResponse{Success: false, Error: err.Error()}, nil
	}

	// 验证文件ID是否匹配
	storedFileID, ok := tokenInfo["file_id"].(string)
	if !ok || storedFileID != req.FileID {
		return &DeleteResponse{Success: false, Error: "令牌与请求文件不匹配"}, nil
	}

	// 删除文件
	err = s.service.Delete(ctx, req.FileID)
	if err != nil {
		return &DeleteResponse{Success: false, Error: err.Error()}, nil
	}
	return &DeleteResponse{Success: true}, nil
}

// InitMultipartUpload 实现InitMultipartUpload RPC方法
func (s *GRPCServer) InitMultipartUpload(ctx context.Context, req *InitMultipartUploadRequest) (*InitMultipartUploadResponse, error) {
	// 验证令牌
	tokenInfo, err := s.service.VerifyToken(ctx, req.Token, "multipart_init")
	if err != nil {
		return &InitMultipartUploadResponse{Error: err.Error()}, nil
	}

	// 验证文件ID是否匹配
	storedFileID, ok := tokenInfo["file_id"].(string)
	if !ok || storedFileID != req.FileID {
		return &InitMultipartUploadResponse{Error: "令牌与请求文件不匹配"}, nil
	}

	// 初始化分片上传
	uploadID, err := s.service.InitMultipartUpload(ctx, req.FileID, req.Filename)
	if err != nil {
		return &InitMultipartUploadResponse{Error: err.Error()}, nil
	}
	return &InitMultipartUploadResponse{UploadID: uploadID}, nil
}

// UploadPart 实现UploadPart RPC方法
func (s *GRPCServer) UploadPart(ctx context.Context, req *UploadPartRequest) (*UploadPartResponse, error) {
	// 验证令牌
	_, err := s.service.VerifyToken(ctx, req.Token, "multipart_upload")
	if err != nil {
		return &UploadPartResponse{Error: err.Error()}, nil
	}

	// 上传分片
	etag, err := s.service.UploadPart(ctx, req.UploadID, int(req.PartNumber), bytes.NewReader(req.Data))
	if err != nil {
		return &UploadPartResponse{Error: err.Error()}, nil
	}
	return &UploadPartResponse{ETag: etag}, nil
}

// CompleteMultipartUpload 实现CompleteMultipartUpload RPC方法
func (s *GRPCServer) CompleteMultipartUpload(ctx context.Context, req *CompleteMultipartUploadRequest) (*CompleteMultipartUploadResponse, error) {
	// 验证令牌
	_, err := s.service.VerifyToken(ctx, req.Token, "multipart_complete")
	if err != nil {
		return &CompleteMultipartUploadResponse{Error: err.Error()}, nil
	}

	// 转换分片信息
	parts := make([]storage.PartInfo, len(req.Parts))
	for i, part := range req.Parts {
		parts[i] = storage.PartInfo{
			PartNumber: int(part.PartNumber),
			ETag:       part.ETag,
		}
	}

	// 完成分片上传
	fileID, err := s.service.CompleteMultipartUpload(ctx, req.UploadID, parts)
	if err != nil {
		return &CompleteMultipartUploadResponse{Error: err.Error()}, nil
	}
	return &CompleteMultipartUploadResponse{FileID: fileID}, nil
}

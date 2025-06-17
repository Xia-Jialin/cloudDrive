package storage

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStorage struct {
	Client *minio.Client
	Bucket string
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
	return &MinioStorage{Client: client, Bucket: bucket}, nil
}

// Storage 接口实现
func (m *MinioStorage) Save(key string, content io.Reader) error {
	_, err := m.Client.PutObject(context.Background(), m.Bucket, key, content, -1, minio.PutObjectOptions{})
	return err
}

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

func (m *MinioStorage) Delete(key string) error {
	return m.Client.RemoveObject(context.Background(), m.Bucket, key, minio.RemoveObjectOptions{})
}

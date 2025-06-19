package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

// 定义一个通用的key/value结构体用于测试
var testFile = FileInfo{
	Key:     "testkey",
	Content: []byte("hello world"),
}

func TestStorageInterface(t *testing.T) {
	dir, err := os.MkdirTemp("", "localfilestorage-test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(dir)

	s := &LocalFileStorage{Dir: dir}
	ctx := context.Background()

	t.Run("上传文件", func(t *testing.T) {
		err := s.Upload(ctx, testFile.Key, bytes.NewReader(testFile.Content))
		if err != nil {
			t.Errorf("上传文件失败: %v", err)
		}
	})

	t.Run("下载文件", func(t *testing.T) {
		reader, err := s.Download(ctx, testFile.Key)
		if err != nil {
			t.Errorf("下载文件失败: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("读取下载内容失败: %v", err)
		}

		if string(data) != string(testFile.Content) {
			t.Errorf("下载内容不符，期望: %s, 实际: %s", testFile.Content, data)
		}
	})

	t.Run("删除文件", func(t *testing.T) {
		err := s.Delete(ctx, testFile.Key)
		if err != nil {
			t.Errorf("删除文件失败: %v", err)
		}
	})
}

// 测试旧版接口兼容性
func TestLegacyStorageInterface(t *testing.T) {
	dir, err := os.MkdirTemp("", "localfilestorage-legacy-test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(dir)

	s := &LocalFileStorage{Dir: dir}

	t.Run("保存key", func(t *testing.T) {
		err := s.Save(testFile.Key, bytes.NewReader(testFile.Content))
		if err != nil {
			t.Errorf("保存key失败: %v", err)
		}
	})

	t.Run("读取key", func(t *testing.T) {
		data, err := s.Read(testFile.Key)
		if err != nil {
			t.Errorf("读取key失败: %v", err)
		}
		if string(data) != string(testFile.Content) {
			t.Errorf("读取内容不符，期望: %s, 实际: %s", testFile.Content, data)
		}
	})
}

package storage

import (
	"bytes"
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

	t.Run("删除key", func(t *testing.T) {
		err := s.Delete(testFile.Key)
		if err != nil {
			t.Errorf("删除key失败: %v", err)
		}
	})
}

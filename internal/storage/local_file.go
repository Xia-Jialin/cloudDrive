package storage

import (
	"io/ioutil"
	"os"
)

// LocalFileStorage 实现 Storage 接口，基于本地文件系统
// key 直接作为文件名（可根据需要加前缀或目录）
type LocalFileStorage struct {
	Dir string // 存储根目录
}

func (l *LocalFileStorage) Save(key string, content []byte) error {
	filePath := l.Dir + "/" + key
	return ioutil.WriteFile(filePath, content, 0644)
}

func (l *LocalFileStorage) Read(key string) ([]byte, error) {
	filePath := l.Dir + "/" + key
	return ioutil.ReadFile(filePath)
}

func (l *LocalFileStorage) Delete(key string) error {
	filePath := l.Dir + "/" + key
	return os.Remove(filePath)
}

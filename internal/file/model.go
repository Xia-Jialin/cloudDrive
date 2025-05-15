package file

import (
	"time"

	"errors"

	"gorm.io/gorm"
)

type FileContent struct {
	Hash string `gorm:"primaryKey;size:64" json:"hash"`
	Size int64  `json:"size"`
	// 可扩展更多内容相关字段，如存储路径等
}

type File struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	Name       string         `gorm:"size:255" json:"name"`
	Hash       string         `gorm:"size:64;index" json:"hash"` // 外键关联 FileContent
	Type       string         `gorm:"size:20" json:"type"`
	ParentID   uint           `json:"parent_id"`
	OwnerID    uint           `json:"owner_id"`
	UploadTime time.Time      `json:"upload_time"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	// 可扩展更多字段，如分享状态、权限等
}

var ErrNameExists = errors.New("同目录下已存在同名文件")

// RenameFile 重命名指定ID的文件，只有所有者可以重命名，且同目录下文件名需唯一
func RenameFile(db *gorm.DB, fileID uint, ownerID uint, newName string) error {
	var f File
	if err := db.First(&f, "id = ?", fileID).Error; err != nil {
		return err
	}
	if f.OwnerID != ownerID {
		return ErrNoPermission
	}
	// 检查同目录下是否有同名文件
	var count int64
	db.Model(&File{}).Where("parent_id = ? AND name = ? AND id != ?", f.ParentID, newName, fileID).Count(&count)
	if count > 0 {
		return ErrNameExists
	}
	return db.Model(&f).Update("name", newName).Error
}

type RenameFileRequest struct {
	NewName string `json:"new_name"`
}

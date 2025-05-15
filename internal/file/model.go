package file

import (
	"time"

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

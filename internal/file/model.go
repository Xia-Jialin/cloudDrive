package file

import (
	"time"
)

type File struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"size:255" json:"name"`
	Size       int64     `json:"size"`
	Type       string    `gorm:"size:20" json:"type"` // file or folder
	ParentID   uint      `json:"parent_id"`           // 父目录ID，根目录为0
	OwnerID    uint      `json:"owner_id"`            // 所有者用户ID
	UploadTime time.Time `json:"upload_time"`
	// 可扩展更多字段，如分享状态、权限等
}

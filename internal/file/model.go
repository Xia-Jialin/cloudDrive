package file

import (
	"time"

	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FileContent struct {
	Hash string `gorm:"primaryKey;size:64" json:"hash"`
	Size int64  `json:"size"`
	// 可扩展更多内容相关字段，如存储路径等
}

type File struct {
	ID         string         `gorm:"type:char(36);primaryKey" json:"id"`
	Name       string         `gorm:"size:255" json:"name"`
	Hash       string         `gorm:"size:64;index" json:"hash"` // 外键关联 FileContent
	Type       string         `gorm:"size:20" json:"type"`
	ParentID   string         `json:"parent_id"`
	OwnerID    uint           `json:"owner_id"`
	UploadTime time.Time      `json:"upload_time"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
	// 可扩展更多字段，如分享状态、权限等
}

func (f *File) BeforeCreate(tx *gorm.DB) (err error) {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return
}

var ErrNameExists = errors.New("同目录下已存在同名文件")

// RenameFile 重命名指定ID的文件，只有所有者可以重命名，且同目录下文件名需唯一
func RenameFile(db *gorm.DB, fileID string, ownerID uint, newName string) error {
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

// CreateFolder 新建文件夹，只有所有者可以新建，且同目录下文件夹名需唯一
func CreateFolder(db *gorm.DB, name, parentID string, ownerID uint) (*File, error) {
	// 检查同目录下是否有同名文件夹
	var count int64
	db.Model(&File{}).Where("parent_id = ? AND name = ? AND type = ?", parentID, name, "folder").Count(&count)
	if count > 0 {
		return nil, ErrNameExists
	}
	folder := &File{
		Name:       name,
		Type:       "folder",
		ParentID:   parentID,
		OwnerID:    ownerID,
		UploadTime: time.Now(),
	}
	if err := db.Create(folder).Error; err != nil {
		return nil, err
	}
	return folder, nil
}

// 用户根目录映射表
// 每个用户有唯一的根目录ID
// UserID为用户ID，RootID为根目录文件夹ID
type UserRoot struct {
	UserID    uint      `gorm:"primaryKey" json:"user_id"`
	RootID    string    `gorm:"type:char(36);uniqueIndex" json:"root_id"`
	CreatedAt time.Time `json:"created_at"`
}

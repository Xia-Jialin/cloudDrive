package file

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNoPermission = errors.New("无权限删除该文件")

// DeleteFile 删除指定ID的文件，只有所有者可以删除
func DeleteFile(db *gorm.DB, fileID string, ownerID uint) error {
	var f File
	if err := db.First(&f, "id = ?", fileID).Error; err != nil {
		return err
	}
	if f.OwnerID != ownerID {
		return ErrNoPermission
	}
	return db.Delete(&f).Error
}

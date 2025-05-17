package file

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNoPermission = errors.New("无权限删除该文件")

// DeleteFile 删除指定ID的文件，只有所有者可以删除
func DeleteFile(db *gorm.DB, fileID string, ownerID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 先删除所有与该文件相关的分享链接
		if err := tx.Where("resource_id = ?", fileID).Delete(&Share{}).Error; err != nil {
			return err
		}

		var f File
		if err := tx.First(&f, "id = ?", fileID).Error; err != nil {
			return err
		}
		if f.OwnerID != ownerID {
			return ErrNoPermission
		}
		if err := tx.Delete(&f).Error; err != nil {
			return err
		}
		return nil
	})
}

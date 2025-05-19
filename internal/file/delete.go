package file

import (
	"errors"
	"fmt"

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

// RestoreFile 恢复回收站中的文件（软删除还原）
func RestoreFile(db *gorm.DB, fileID string, ownerID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var f File
		if err := tx.Unscoped().First(&f, "id = ?", fileID).Error; err != nil {
			fmt.Printf("[RestoreFile] 查询文件失败 fileID=%s, ownerID=%d, err=%v\n", fileID, ownerID, err)
			return err
		}
		if f.OwnerID != ownerID {
			fmt.Printf("[RestoreFile] 无权限还原 fileID=%s, ownerID=%d, 文件owner=%d\n", fileID, ownerID, f.OwnerID)
			return ErrNoPermission
		}
		if f.DeletedAt.Valid {
			res := tx.Unscoped().Model(&File{}).
				Where("id = ? AND owner_id = ?", fileID, ownerID).
				Update("deleted_at", nil)
			fmt.Printf("[RestoreFile] 还原文件 fileID=%s, ownerID=%d, updateErr=%v, rowsAffected=%d\n", fileID, ownerID, res.Error, res.RowsAffected)
			return res.Error
		}
		fmt.Printf("[RestoreFile] 文件已是正常状态 fileID=%s, ownerID=%d\n", fileID, ownerID)
		return nil // 已是正常状态
	})
}

// PermanentlyDeleteFile 彻底删除回收站中的文件（物理删除）
func PermanentlyDeleteFile(db *gorm.DB, fileID string, ownerID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var f File
		if err := tx.Unscoped().First(&f, "id = ?", fileID).Error; err != nil {
			return err
		}
		if f.OwnerID != ownerID {
			return ErrNoPermission
		}
		// 先删除所有与该文件相关的分享链接
		if err := tx.Where("resource_id = ?", fileID).Delete(&Share{}).Error; err != nil {
			return err
		}
		// 物理删除文件元数据
		if err := tx.Unscoped().Delete(&f).Error; err != nil {
			return err
		}
		return nil
	})
}

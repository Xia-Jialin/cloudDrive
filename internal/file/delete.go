package file

import (
	"errors"

	"gorm.io/gorm"
)

var ErrNoPermission = errors.New("无权限删除该文件")
var ErrRestoreParentNotExist = errors.New("原路径不存在，请选择新的还原路径")

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

// RestoreFile 恢复回收站中的文件（软删除还原），可指定新还原路径
func RestoreFile(db *gorm.DB, fileID string, ownerID uint, targetParentID ...string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var f File
		if err := tx.Unscoped().First(&f, "id = ?", fileID).Error; err != nil {
			return err
		}
		if f.OwnerID != ownerID {
			return ErrNoPermission
		}
		// 选择目标目录
		parentID := f.ParentID
		if len(targetParentID) > 0 && targetParentID[0] != "" {
			parentID = targetParentID[0]
		}
		// 校验父目录是否存在且未被软删除
		if parentID != "" {
			var parent File
			if err := tx.Unscoped().First(&parent, "id = ?", parentID).Error; err != nil {
				return ErrRestoreParentNotExist
			}
			if parent.DeletedAt.Valid {
				return ErrRestoreParentNotExist
			}
		}
		// 校验新目录下无同名文件/文件夹
		var count int64
		tx.Model(&File{}).Where("parent_id = ? AND name = ? AND id != ?", parentID, f.Name, f.ID).Count(&count)
		if count > 0 {
			return ErrNameExists
		}
		if f.DeletedAt.Valid {
			res := tx.Unscoped().Model(&File{}).
				Where("id = ? AND owner_id = ?", fileID, ownerID).
				Updates(map[string]interface{}{"deleted_at": nil, "parent_id": parentID})
			return res.Error
		}
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

package file

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}
	if err := db.AutoMigrate(&File{}, &FileContent{}, &UserRoot{}, &Share{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestDeleteFile_MoveToRecycleBin(t *testing.T) {
	db := setupTestDB(t)
	// 创建用户根目录
	userID := uint(1)
	root := &UserRoot{UserID: userID, RootID: "root", CreatedAt: time.Now()}
	db.Create(root)
	// 创建文件
	file := &File{
		ID:         "file1",
		Name:       "test.txt",
		Type:       "file",
		ParentID:   root.RootID,
		OwnerID:    userID,
		UploadTime: time.Now(),
	}
	db.Create(file)
	// 删除文件（应为软删除）
	err := DeleteFile(db, file.ID, userID)
	if err != nil {
		t.Fatalf("delete file failed: %v", err)
	}
	// 检查文件是否软删除（gorm的DeletedAt应不为零）
	var deleted File
	db.Unscoped().First(&deleted, "id = ?", file.ID)
	if deleted.DeletedAt.Time.IsZero() {
		t.Errorf("file should be soft deleted (DeletedAt should be set)")
	}
}

func TestListRecycleBinFiles(t *testing.T) {
	db := setupTestDB(t)
	userID := uint(2)
	root := &UserRoot{UserID: userID, RootID: "root2", CreatedAt: time.Now()}
	db.Create(root)
	// 创建两个文件，一个正常，一个软删除
	file1 := &File{ID: "f1", Name: "a.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	file2 := &File{ID: "f2", Name: "b.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	db.Create(file1)
	db.Create(file2)
	db.Delete(file2) // 软删除b.txt

	// 查询回收站文件（只应返回被软删除的文件）
	files, err := ListRecycleBinFiles(db, userID, 1, 10)
	if err != nil {
		t.Fatalf("list recycle bin files failed: %v", err)
	}
	if len(files) != 1 || files[0].ID != "f2" {
		t.Errorf("recycle bin should only contain soft deleted files, got: %+v", files)
	}
}

func TestRestoreFileFromRecycleBin(t *testing.T) {
	db := setupTestDB(t)
	userID := uint(3)
	root := &UserRoot{UserID: userID, RootID: "root3", CreatedAt: time.Now()}
	db.Create(root)
	file := &File{ID: "f3", Name: "c.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	db.Create(file)
	db.Delete(file) // 软删除

	// 恢复文件
	err := RestoreFile(db, file.ID, userID)
	if err != nil {
		t.Fatalf("restore file failed: %v", err)
	}
	// 检查文件是否已恢复（DeletedAt应为零）
	var restored File
	db.First(&restored, "id = ?", file.ID)
	if restored.DeletedAt.Valid {
		t.Errorf("file should be restored (DeletedAt should be zero)")
	}
}

func TestPermanentlyDeleteFileFromRecycleBin(t *testing.T) {
	db := setupTestDB(t)
	userID := uint(4)
	root := &UserRoot{UserID: userID, RootID: "root4", CreatedAt: time.Now()}
	db.Create(root)
	file := &File{ID: "f4", Name: "d.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	db.Create(file)
	db.Delete(file) // 软删除

	// 彻底删除
	err := PermanentlyDeleteFile(db, file.ID, userID)
	if err != nil {
		t.Fatalf("permanently delete file failed: %v", err)
	}
	// 检查文件是否已彻底删除
	var f File
	db.Unscoped().First(&f, "id = ?", file.ID)
	if f.ID != "" {
		t.Errorf("file should be permanently deleted, got: %+v", f)
	}
}

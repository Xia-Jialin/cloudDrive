package handler

import (
	"bytes"
	"cloudDrive/internal/file"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRecycleTestRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/recycle/restore", func(c *gin.Context) {
		c.Set("db", db)
		c.Set("user_id", userID)
		RecycleBinRestoreHandler(c)
	})
	return r
}

func TestRecycleBinRestoreHandler_Success(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&file.File{}, &file.UserRoot{})
	userID := uint(1)
	root := &file.UserRoot{UserID: userID, RootID: "root", CreatedAt: time.Now()}
	db.Create(root)
	f := &file.File{ID: "f1", Name: "test.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	db.Create(f)
	db.Delete(f) // 软删除

	r := setupRecycleTestRouter(db, userID)
	body := map[string]interface{}{"file_id": f.ID}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/recycle/restore", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "还原成功" {
		t.Errorf("expected 还原成功, got %v", resp["message"])
	}
	var restored file.File
	db.First(&restored, "id = ?", f.ID)
	if restored.DeletedAt.Valid {
		t.Errorf("file should be restored (DeletedAt should be NULL)")
	}
}

func TestRecycleBinRestoreHandler_NoPermission(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&file.File{}, &file.UserRoot{})
	userID := uint(1)
	root := &file.UserRoot{UserID: userID, RootID: "root", CreatedAt: time.Now()}
	db.Create(root)
	f := &file.File{ID: "f2", Name: "test2.txt", Type: "file", ParentID: root.RootID, OwnerID: userID, UploadTime: time.Now()}
	db.Create(f)
	db.Delete(f)

	r := setupRecycleTestRouter(db, userID+1) // 非拥有者
	body := map[string]interface{}{"file_id": f.ID}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/recycle/restore", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == nil || resp["error"].(string) == "" {
		t.Errorf("expected error message, got %v", resp["error"])
	}
}

func TestRecycleBinRestoreHandler_BadRequest(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&file.File{}, &file.UserRoot{})
	r := setupRecycleTestRouter(db, 1)
	// 缺少 file_id
	body := map[string]interface{}{"target_path": ""}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/recycle/restore", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == nil || resp["error"].(string) == "" {
		t.Errorf("expected error message, got %v", resp["error"])
	}
}

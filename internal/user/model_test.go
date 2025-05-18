package user

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUser_CheckPassword(t *testing.T) {
	plainPassword := "123456"
	hash, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("生成hash失败: %v", err)
	}
	user := User{Password: string(hash)}
	t.Run("正确密码", func(t *testing.T) {
		if err := user.CheckPassword(plainPassword); err != nil {
			t.Errorf("期望密码校验通过，但失败: %v", err)
		}
	})

	t.Run("错误密码", func(t *testing.T) {
		if err := user.CheckPassword("wrongpass"); err == nil {
			t.Errorf("期望密码校验失败，但通过了")
		}
	})
}

func TestGetUserByIDAndUpdateUserStorageUsed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	db.AutoMigrate(&User{})
	user := User{Username: "testuser", Password: "123456", StorageLimit: 100, StorageUsed: 10}
	db.Create(&user)

	t.Run("GetUserByID 正常获取", func(t *testing.T) {
		u, err := GetUserByID(db, user.ID)
		if err != nil {
			t.Errorf("期望无错误，实际: %v", err)
		}
		if u.Username != "testuser" {
			t.Errorf("期望用户名 testuser，实际: %s", u.Username)
		}
	})

	t.Run("UpdateUserStorageUsed 正常更新", func(t *testing.T) {
		err := UpdateUserStorageUsed(db, user.ID, 20)
		if err != nil {
			t.Errorf("期望无错误，实际: %v", err)
		}
		u, _ := GetUserByID(db, user.ID)
		if u.StorageUsed != 30 {
			t.Errorf("期望StorageUsed为30，实际: %d", u.StorageUsed)
		}
	})

	t.Run("GetUserByID 用户不存在", func(t *testing.T) {
		_, err := GetUserByID(db, 9999)
		if err == nil {
			t.Errorf("期望报错，实际无错误")
		}
	})
}

func TestCreateUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	db.AutoMigrate(&User{})
	user := User{Username: "newuser", Password: "hashedpwd", StorageLimit: 100, StorageUsed: 0}
	err = CreateUser(db, &user)
	if err != nil {
		t.Errorf("期望无错误，实际: %v", err)
	}
	var got User
	err = db.First(&got, "username = ?", "newuser").Error
	if err != nil {
		t.Errorf("插入后未找到用户: %v", err)
	}
	if got.Username != "newuser" || got.Password != "hashedpwd" {
		t.Errorf("插入用户数据不符: %+v", got)
	}
}

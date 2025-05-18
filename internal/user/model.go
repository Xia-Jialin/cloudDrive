package user

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"uniqueIndex;size:50" json:"username"`
	Password     string         `gorm:"size:255" json:"-"`
	Nickname     string         `gorm:"size:50" json:"nickname"`
	Avatar       string         `gorm:"size:255" json:"avatar"`
	Role         string         `gorm:"size:20;default:user" json:"role"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	StorageLimit int64          `gorm:"default:1073741824" json:"storage_limit"` // 1G 默认
	StorageUsed  int64          `gorm:"default:0" json:"storage_used"`           // 已用空间
}

func (u *User) CheckPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}

// GetUserByID 根据用户ID获取用户信息
func GetUserByID(db *gorm.DB, userID uint) (*User, error) {
	var user User
	err := db.First(&user, userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUserStorageUsed 更新用户已用存储空间
func UpdateUserStorageUsed(db *gorm.DB, userID uint, delta int64) error {
	return db.Model(&User{}).Where("id = ?", userID).UpdateColumn("storage_used", gorm.Expr("storage_used + ?", delta)).Error
}

// CreateUser 插入新用户（不做密码加密，需外部保证密码已加密）
func CreateUser(db *gorm.DB, user *User) error {
	return db.Create(user).Error
}

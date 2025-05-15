package user

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Username  string         `gorm:"uniqueIndex;size:50" json:"username"`
	Password  string         `gorm:"size:255" json:"-"`
	Nickname  string         `gorm:"size:50" json:"nickname"`
	Avatar    string         `gorm:"size:255" json:"avatar"`
	Role      string         `gorm:"size:20;default:user" json:"role"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (u *User) CheckPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
}

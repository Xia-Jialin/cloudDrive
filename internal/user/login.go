package user

import (
	"errors"
	"log"

	"gorm.io/gorm"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
}

type LoginResponse struct {
	User User `json:"user"`
}

func Login(db *gorm.DB, req LoginRequest) (*LoginResponse, error) {
	var user User
	db.Where("username = ?", req.Username).First(&user)
	if user.ID == 0 {
		return nil, errors.New("用户不存在")
	}
	if err := user.CheckPassword(req.Password); err != nil {
		log.Printf("[DEBUG] 登录用户名: %s, 数据库密码hash: %s, 前端密码: %s", req.Username, user.Password, req.Password)
		return nil, errors.New("密码错误")
	}
	user.Password = ""
	return &LoginResponse{User: user}, nil
}

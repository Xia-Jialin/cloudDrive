package user

import (
	"errors"
	"log"
	"os"
	"time"

	"gorm.io/gorm"

	"github.com/golang-jwt/jwt/v5"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Claims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
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
	// 生成JWT Token
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "cloudDriveSecret"
	}
	claims := Claims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secret))
	if err != nil {
		return nil, errors.New("Token生成失败")
	}
	user.Password = ""
	return &LoginResponse{Token: tokenStr, User: user}, nil
}

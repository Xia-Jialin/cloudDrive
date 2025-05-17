package user

import (
	"errors"
	"log"

	"cloudDrive/internal/file"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
}

type RegisterResponse struct {
	ID uint `json:"id"`
}

func isPasswordComplex(password string) bool {
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range password {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case (c >= 33 && c <= 47) || (c >= 58 && c <= 64) || (c >= 91 && c <= 96) || (c >= 123 && c <= 126):
			hasSpecial = true
		}
	}
	count := 0
	if hasUpper {
		count++
	}
	if hasLower {
		count++
	}
	if hasDigit {
		count++
	}
	if hasSpecial {
		count++
	}
	return count >= 3
}

func Register(db *gorm.DB, req RegisterRequest) (*RegisterResponse, error) {
	log.Printf("[DEBUG] Register called with username: %s", req.Username)
	if req.Username == "" {
		log.Printf("[ERROR] 用户名为空")
		return nil, errors.New("用户名不能为空")
	}
	if !isPasswordComplex(req.Password) {
		return nil, errors.New("密码必须包含大写字母、小写字母、数字、特殊字符中的至少三种")
	}
	// 检查用户名是否已存在
	var count int64
	db.Model(&User{}).Where("username = ?", req.Username).Count(&count)
	log.Printf("[DEBUG] 用户名 %s 已存在数量: %d", req.Username, count)
	if count > 0 {
		log.Printf("[ERROR] 用户名已注册: %s", req.Username)
		return nil, errors.New("用户名已注册")
	}
	// 密码加密
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] 密码加密失败: %v", err)
		return nil, errors.New("密码加密失败")
	}
	user := User{
		Username:     req.Username,
		Password:     string(hash),
		Nickname:     GenerateNickname(),
		StorageLimit: 1073741824, // 1G
		StorageUsed:  0,
	}
	if err := db.Create(&user).Error; err != nil {
		log.Printf("[ERROR] 用户创建失败: %v", err)
		return nil, err
	}

	// 创建根目录文件夹
	rootFolder := &file.File{
		Name:       "根目录",
		Type:       "folder",
		ParentID:   "", // 根目录的ParentID为空
		OwnerID:    user.ID,
		UploadTime: user.CreatedAt,
	}
	if err := db.Create(rootFolder).Error; err != nil {
		log.Printf("[ERROR] 根目录创建失败: %v", err)
		return nil, err
	}

	// 记录用户根目录映射
	userRoot := &file.UserRoot{
		UserID:    user.ID,
		RootID:    rootFolder.ID,
		CreatedAt: user.CreatedAt,
	}
	if err := db.Create(userRoot).Error; err != nil {
		log.Printf("[ERROR] UserRoot创建失败: %v", err)
		return nil, err
	}

	log.Printf("[DEBUG] 用户注册成功: id=%d, username=%s", user.ID, user.Username)
	return &RegisterResponse{ID: user.ID}, nil
}

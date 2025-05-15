package user

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
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

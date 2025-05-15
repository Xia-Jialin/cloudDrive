package user

import (
	"fmt"
	"math/rand"
)

func GenerateNickname() string {
	return fmt.Sprintf("用户%d", rand.Intn(1000000))
}

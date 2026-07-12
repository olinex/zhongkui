// Package crypto 提供密码学相关的工具函数，包括密码哈希生成与校验等功能。
package crypto

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 使用 bcrypt 算法对原始密码进行哈希处理。
//
// bcrypt 是一种自适应的密码哈希算法，内置盐值（salt），能够有效抵御彩虹表攻击。
// 该函数使用 bcrypt 的默认成本因子（DefaultCost=10）来平衡安全性与性能。
//
// 参数：
//   - password: 用户的原始明文密码
//
// 返回值：
//   - []byte: 经过 bcrypt 哈希处理后的密码字节数组，可直接存储到数据库
//   - error: 如果哈希过程中发生错误（如密码长度超过 72 字节限制），则返回非 nil 错误
func HashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// CheckPassword 校验原始密码与哈希密码是否匹配。
//
// 该函数通过 bcrypt 的安全比较机制来验证密码，用于用户登录等身份认证场景。
// 即使密码不匹配，也不会泄露关于哈希值或密码的具体信息，可防止时序攻击。
//
// 参数：
//   - hashedPassword: 之前通过 HashPassword 生成的哈希密码（通常从数据库中读取）
//   - rawPassword: 用户输入的原始明文密码
//
// 返回值：
//   - bool: 如果原始密码与哈希密码匹配则返回 true，否则返回 false
func CheckPassword(hashedPassword []byte, rawPassword string) bool {
	return bcrypt.CompareHashAndPassword(hashedPassword, []byte(rawPassword)) == nil
}

// Package crypto 提供通用的加密相关工具函数，包括随机字符串生成、密码哈希、签名与验签等功能。
package signature

import (
	"fmt"
	"strings"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// SignFunc 定义签名函数的类型，接收密钥和待签名的原始值，返回签名结果字符串或错误。
// 该类型允许通过依赖注入的方式支持不同的签名算法实现。
type SignFunc func(secret, value string) (string, error)

// Signaturer 是签名器结构体，封装了签名算法的前缀标识、密钥和签名函数，
// 提供统一的签名生成（Sign）和签名校验（Check）能力。
type Signaturer struct {
	prefix string   // 签名算法的前缀标识，用于签名结果格式化和校验时匹配
	secret string   // 签名密钥，用于 HMAC 等签名算法计算
	f      SignFunc // 具体的签名函数，由构造时注入
}

// newSignaturer 创建一个新的签名器实例。
// prefix: 签名算法的前缀标识，用于签名结果的格式化（如 "hmac-sha256"）
// secret: 签名密钥
// f: 具体的签名函数实现
func newSignaturer(prefix, secret string, f SignFunc) *Signaturer {
	return &Signaturer{
		prefix: prefix,
		secret: secret,
		f:      f,
	}
}

// Sign 对原始值进行签名，返回格式为 "<prefix>:<签名值>" 的签名字符串。
// value: 待签名的原始值
// 返回值: 格式化后的签名字符串，或签名过程中产生的错误
func (s *Signaturer) Sign(value string) (string, error) {
	signature, err := s.f(s.secret, value)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return fmt.Sprintf(`%s:%s`, s.prefix, signature), nil
}

// Check 校验给定的签名字符串是否与原始值匹配。
// 校验流程：
//  1. 按 ":" 分割签名字符串，确认格式为 "<prefix>:<签名值>"
//  2. 校验前缀是否与签名器的前缀一致
//  3. 使用签名器的密钥和签名函数对原始值重新计算签名
//  4. 比较计算结果与签名值是否一致
//
// signature: 待校验的签名字符串，格式应为 "<prefix>:<签名值>"
// raw: 原始值
// 返回值: 签名是否有效（bool），以及校验过程中可能产生的错误
func (s *Signaturer) Check(signature, raw string) (bool, error) {
	parts := strings.Split(signature, ":")
	if len(parts) != 2 {
		return false, nil
	}
	if parts[0] != s.prefix {
		return false, nil
	}
	check, err := s.f(s.secret, raw)
	if err != nil {
		return false, errors.WithStack(err)
	}
	if check != parts[1] {
		return false, nil
	}
	return true, nil
}

package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// PrefixHmacsha256 定义 HMAC-SHA256 签名算法的前缀标识，用于签名值的格式前缀，
// 签名结果格式为 "hmac-sha256:<base64编码的签名值>"
const (
	PrefixHmacsha256 string = "hmac-sha256"
)

// HMACSHA256Sign 使用 HMAC-SHA256 算法对值进行签名。
// secret: HMAC 密钥
// value: 待签名的原始值
// 返回值: 经 Base64 标准编码的签名字符串，或签名过程中产生的错误
func HMACSHA256Sign(secret, value string) (string, error) {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(value))
	return base64.StdEncoding.EncodeToString(h.Sum(nil)), nil
}

// NewHMACSHA256Signaturer 创建一个使用 HMAC-SHA256 算法的签名器实例。
// secret: 签名密钥
// 返回值: 配置好的 Signaturer 指针，可直接调用 Sign/Check 方法进行签名和校验
func NewHMACSHA256Signaturer(secret string) *Signaturer {
	return newSignaturer(PrefixHmacsha256, secret, HMACSHA256Sign)
}

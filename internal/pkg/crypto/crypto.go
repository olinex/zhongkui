// Package crypto 提供密码学相关的工具函数，包括随机字符串生成等功能。
// 本包使用 crypto/rand 作为随机数源，确保生成的随机值具备密码学安全性，
// 适用于令牌生成、密钥派生、会话标识符等安全敏感场景。
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// charset 定义了 GenerateRandomString 函数使用的字符集，
// 包含26个小写字母、26个大写字母和10个数字，共计62个字符。
// 该字符集仅包含字母和数字，不包含特殊字符，
// 适用于需要避免转义或编码问题的场景（如文件名、URL路径段等）。
const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateRandomString 生成指定长度的随机字符串。
//
// 该函数使用 crypto/rand 密码学安全随机数生成器，从 charset 字符集中
// 逐个字符随机选取，拼接成指定长度的字符串。每个字符的选取相互独立，
// 服从均匀分布，确保生成的字符串具有良好的随机性和不可预测性。
//
// 参数：
//   - length: 期望生成的字符串长度。若为0，则返回空字符串。
//
// 返回值：
//   - string: 生成的随机字符串，仅包含字母和数字（a-z, A-Z, 0-9）。
//   - error: 如果随机数生成失败则返回错误，否则返回nil。
//
// 注意：由于每个字符独立从62个字符中选取，较短字符串的字符多样性有限。
// 如需更高的信息密度或更短的等价随机值，请考虑使用 GenerateRandomBase64String。
func GenerateRandomString(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	bytes := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range bytes {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", errors.WithStack(err)
		}
		bytes[i] = charset[num.Int64()]
	}
	return string(bytes), nil
}

// GenerateRandomBase64String 生成指定长度的随机Base64字符串。
//
// 该函数先使用 crypto/rand 生成密码学安全的随机字节序列，
// 再通过URL安全的Base64编码（URLEncoding）转换为字符串，
// 并去除末尾的 padding 字符（=），使输出更简洁。
//
// URL安全Base64编码使用 '-' 替代 '+'，'_' 替代 '/'，
// 因此生成的字符串可以安全地嵌入URL、文件名、HTTP Header等场景，
// 无需额外的URL编码处理。
//
// 参数：
//   - length: 期望生成的Base64字符串长度。若为0，则返回空字符串。
//     注意：由于Base64编码每3字节产生4个字符，当 length 不是4的倍数时，
//     实际输出长度可能略小于指定长度（向下取整到最近的4的倍数）。
//
// 返回值：
//   - string: 生成的URL安全Base64随机字符串，不包含 padding。
//   - error: 如果随机数生成失败则返回错误，否则返回nil。
//
// 相比 GenerateRandomString，该函数在相同输出长度下提供更高的信息熵
// （每个字符约6比特 vs 每个字符约5.95比特），且包含更多种类的字符。
func GenerateRandomBase64String(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	bytes := make([]byte, (length*3)/4)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.WithStack(err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes), nil
}

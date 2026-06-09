package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"io"
	"os"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// CalculateFileMD5 计算文件的 MD5 哈希值
//
// 该函数打开指定的文件并计算其 MD5 哈希值。MD5（Message Digest Algorithm 5）是一种广泛使用的
// 密码散列函数，可以生成一个 128 位（16 字节）的哈希值，通常用于验证文件完整性。
//
// 参数:
//
//	filePath - 要计算哈希的文件路径
//
// 返回值:
//
//	[]byte - 16 字节的 MD5 哈希值，如果成功
//	error  - 如果打开文件或读取文件时发生错误
//
// 示例:
//
//	hash, err := CalculateFileMD5("/path/to/file.txt")
//	if err != nil {
//	    // 处理错误
//	}
//	fmt.Printf("MD5: %x\n", hash)
func CalculateFileMD5(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, errors.WithStack(err)
	}
	return hash.Sum(nil), nil
}

// CalculateFileSHA256 计算文件的 SHA-256 哈希值
//
// 该函数打开指定的文件并计算其 SHA-256 哈希值。SHA-256（Secure Hash Algorithm 256-bit）
// 是 SHA-2 系列中的一种密码散列函数，可以生成一个 256 位（32 字节）的哈希值。
// 与 MD5 相比，SHA-256 提供更强的安全性，常用于数字签名和证书验证。
//
// 参数:
//
//	filePath - 要计算哈希的文件路径
//
// 返回值:
//
//	[]byte - 32 字节的 SHA-256 哈希值，如果成功
//	error  - 如果打开文件或读取文件时发生错误
//
// 示例:
//
//	hash, err := CalculateFileSHA256("/path/to/file.txt")
//	if err != nil {
//	    // 处理错误
//	}
//	fmt.Printf("SHA-256: %x\n", hash)
func CalculateFileSHA256(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, errors.WithStack(err)
	}
	return hash.Sum(nil), nil
}

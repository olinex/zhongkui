package key

import (
	_ "embed"
	"fmt"
	"os"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// PASSWORD_BOOK_SIZE 定义密码本的固定大小（字节数）
// 65536 字节 = 64KB，提供足够的熵用于密钥派生
const PASSWORD_BOOK_SIZE = 65536

//go:embed default_password_book.bin
// defaultPasswordBookData 嵌入的默认密码本数据
// 该文件在编译时被嵌入到二进制文件中，作为后备方案
var defaultPasswordBookData []byte

// PasswordBook 密码本，提供用于密钥派生的盐值数据。
//
// 密码本是一个固定大小（65536 字节）的随机数据文件，用于：
//   - 为密钥派生提供高质量的盐值（salt）
//   - 通过不同的偏移位置派生出不同的子密钥
//   - 增加密钥派生的随机性和安全性
//
// 密码本应妥善保管，避免泄露。生产环境应使用独立的密码本文件，
// 而不是使用内置的默认密码本。
type PasswordBook struct {
	// data 密码本的原始数据
	data []byte
}

// Bytes 返回密码本的原始字节数据
func (p *PasswordBook) Bytes() []byte {
	return p.data
}

// Len 返回密码本的长度（字节数）
func (p *PasswordBook) Len() int {
	return len(p.data)
}

// LoadPasswordBookFromFile 从文件加载密码本。
//
// 参数：
//   - path: 密码本文件路径
//
// 返回值：
//   - *PasswordBook: 加载的密码本实例
//   - error: 文件不存在、大小不匹配或其他读取错误时返回错误
//
// 错误场景：
//   - 文件不存在或无法读取
//   - 文件大小不等于 PASSWORD_BOOK_SIZE（65536 字节）
func LoadPasswordBookFromFile(path string) (*PasswordBook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(data) != PASSWORD_BOOK_SIZE {
		return nil, errors.Newf("password book file size mismatch: expected %d bytes, got %d bytes", PASSWORD_BOOK_SIZE, len(data))
	}
	return &PasswordBook{data: data}, nil
}

// LoadPasswordBook 加载密码本，支持回退到内置默认密码本。
//
// 加载逻辑：
//  1. 如果 path 不为空，尝试从文件加载
//  2. 如果文件加载成功，返回文件密码本
//  3. 如果文件不存在（os.IsNotExist），回退到内置密码本
//  4. 如果文件存在但读取失败（如大小不匹配），返回错误
//  5. 如果 path 为空，直接使用内置密码本
//
// 参数：
//   - path: 密码本文件路径，为空时使用内置默认密码本
//
// 返回值：
//   - *PasswordBook: 加载的密码本实例
//   - error: 加载失败时返回错误
//
// 注意：
//   - 使用内置密码本时会输出警告信息到 stderr
//   - 生产环境应使用独立的密码本文件，而非内置密码本
func LoadPasswordBook(path string) (*PasswordBook, error) {
	if path != "" {
		pb, err := LoadPasswordBookFromFile(path)
		if err == nil {
			return pb, nil
		}
		// 如果文件不存在，回退到内置密码本
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			return nil, errors.WithStack(err)
		}
	}

	pb, err := loadEmbeddedPasswordBook()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	fmt.Fprintf(os.Stderr, "WARNING: Using embedded default password book. This is NOT safe for production. Please run 'zki create password-book' to generate your own password book file.\n")
	return pb, nil
}

// loadEmbeddedPasswordBook 加载内置的默认密码本。
//
// 返回值：
//   - *PasswordBook: 内置密码本实例
//   - error: 内置密码本大小不匹配时返回错误
func loadEmbeddedPasswordBook() (*PasswordBook, error) {
	if len(defaultPasswordBookData) != PASSWORD_BOOK_SIZE {
		return nil, errors.Newf("embedded password book size mismatch: expected %d bytes, got %d bytes", PASSWORD_BOOK_SIZE, len(defaultPasswordBookData))
	}
	return &PasswordBook{data: defaultPasswordBookData}, nil
}

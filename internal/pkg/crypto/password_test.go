package crypto

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Run("正常密码哈希", func(t *testing.T) {
		password := "mySecret123"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 返回未预期的错误: %v", err)
		}
		if len(hashed) == 0 {
			t.Fatal("HashPassword() 返回了空的哈希值")
		}
		if string(hashed) == password {
			t.Fatal("HashPassword() 返回的哈希值不应与原始密码相同")
		}
	})

	t.Run("相同密码每次哈希结果不同", func(t *testing.T) {
		password := "samePassword"
		hash1, err := HashPassword(password)
		if err != nil {
			t.Fatalf("第一次哈希失败: %v", err)
		}
		hash2, err := HashPassword(password)
		if err != nil {
			t.Fatalf("第二次哈希失败: %v", err)
		}
		if string(hash1) == string(hash2) {
			t.Fatal("bcrypt 内置盐值，相同密码的两次哈希结果不应相同")
		}
	})

	t.Run("空密码哈希", func(t *testing.T) {
		hashed, err := HashPassword("")
		if err != nil {
			t.Fatalf("HashPassword() 对空密码返回了未预期的错误: %v", err)
		}
		if len(hashed) == 0 {
			t.Fatal("HashPassword() 对空密码返回了空的哈希值")
		}
	})

	t.Run("包含特殊字符的密码", func(t *testing.T) {
		password := "p@$$w0rd!#%^&*()_+-=[]{}|;':\",./<>?"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 对特殊字符密码返回了未预期的错误: %v", err)
		}
		if !CheckPassword(hashed, password) {
			t.Fatal("哈希后的密码应能与原始特殊字符密码匹配")
		}
	})

	t.Run("包含中文的密码", func(t *testing.T) {
		password := "密码测试123"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 对中文密码返回了未预期的错误: %v", err)
		}
		if !CheckPassword(hashed, password) {
			t.Fatal("哈希后的密码应能与原始中文密码匹配")
		}
	})

	t.Run("超过72字节的密码应返回错误", func(t *testing.T) {
		password := "a_password_that_is_definitely_longer_than_seventy_two_bytes_in_total_length_okay"
		_, err := HashPassword(password)
		if err == nil {
			t.Fatal("超过 72 字节的密码应返回错误，但得到了 nil")
		}
	})
}

func TestCheckPassword(t *testing.T) {
	t.Run("密码匹配", func(t *testing.T) {
		password := "correctPassword"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 失败: %v", err)
		}
		if !CheckPassword(hashed, password) {
			t.Fatal("CheckPassword() 对正确密码应返回 true")
		}
	})

	t.Run("密码不匹配", func(t *testing.T) {
		password := "correctPassword"
		wrongPassword := "wrongPassword"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 失败: %v", err)
		}
		if CheckPassword(hashed, wrongPassword) {
			t.Fatal("CheckPassword() 对错误密码应返回 false")
		}
	})

	t.Run("空密码与空哈希匹配", func(t *testing.T) {
		hashed, err := HashPassword("")
		if err != nil {
			t.Fatalf("HashPassword() 对空密码失败: %v", err)
		}
		if !CheckPassword(hashed, "") {
			t.Fatal("CheckPassword() 对空密码的哈希应返回 true")
		}
	})

	t.Run("空密码与非空密码哈希不匹配", func(t *testing.T) {
		password := "notEmpty"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 失败: %v", err)
		}
		if CheckPassword(hashed, "") {
			t.Fatal("CheckPassword() 对空密码与非空密码哈希应返回 false")
		}
	})

	t.Run("大小写敏感", func(t *testing.T) {
		password := "CaseSensitive"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 失败: %v", err)
		}
		if CheckPassword(hashed, "casesensitive") {
			t.Fatal("CheckPassword() 应区分大小写")
		}
		if CheckPassword(hashed, "CASESENSITIVE") {
			t.Fatal("CheckPassword() 应区分大小写")
		}
	})

	t.Run("无效的哈希值", func(t *testing.T) {
		invalidHash := []byte("this_is_not_a_valid_bcrypt_hash")
		if CheckPassword(invalidHash, "anyPassword") {
			t.Fatal("CheckPassword() 对无效哈希值应返回 false")
		}
	})

	t.Run("空白字符差异", func(t *testing.T) {
		password := "password"
		hashed, err := HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword() 失败: %v", err)
		}
		if CheckPassword(hashed, " password") {
			t.Fatal("带前导空格的密码不应与原始密码匹配")
		}
		if CheckPassword(hashed, "password ") {
			t.Fatal("带末尾空格的密码不应与原始密码匹配")
		}
	})
}

func TestHashPasswordAndCheckPasswordIntegration(t *testing.T) {
	t.Run("哈希与校验完整流程", func(t *testing.T) {
		passwords := []string{
			"simple",
			"with spaces",
			"p@ss!w0rd",
			"中文密码",
			"a",
			"12345678",
		}
		for _, pwd := range passwords {
			hashed, err := HashPassword(pwd)
			if err != nil {
				t.Errorf("密码 %q 哈希失败: %v", pwd, err)
				continue
			}
			if !CheckPassword(hashed, pwd) {
				t.Errorf("密码 %q 哈希后校验失败", pwd)
			}
		}
	})
}

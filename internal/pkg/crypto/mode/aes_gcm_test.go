package mode

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// ---------------------------------------------------------------------------
// NewAESGCMMode 构造函数测试
// ---------------------------------------------------------------------------

func TestNewAESGCMMode_ValidKeyLengths(t *testing.T) {
	testCases := []struct {
		name    string
		keyLen  int
		wantErr bool
	}{
		{"AES-128", 16, false},
		{"AES-192", 24, false},
		{"AES-256", 32, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := make([]byte, tc.keyLen)
			rand.Read(key)

			mode, err := NewAESGCMMode(key, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode == nil {
				t.Fatal("expected non-nil mode")
			}
		})
	}
}

func TestNewAESGCMMode_InvalidKeyLengths(t *testing.T) {
	testCases := []int{0, 1, 8, 15, 17, 20, 23, 25, 31, 33, 64}

	for _, keyLen := range testCases {
		t.Run(string(rune(keyLen)), func(t *testing.T) {
			key := make([]byte, keyLen)
			_, err := NewAESGCMMode(key, nil)
			if err == nil {
				t.Fatalf("expected error for key length %d, got nil", keyLen)
			}
		})
	}
}

func TestNewAESGCMMode_WithAdditionalData(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	aad := []byte("additional authenticated data")

	mode, err := NewAESGCMMode(key, aad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode == nil {
		t.Fatal("expected non-nil mode")
	}
	if !bytes.Equal(mode.additionalData, aad) {
		t.Fatal("additional data not stored correctly")
	}
}

// ---------------------------------------------------------------------------
// Encrypt/Decrypt 基本功能测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, err := NewAESGCMMode(key, nil)
	if err != nil {
		t.Fatalf("failed to create mode: %v", err)
	}

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short text", []byte("hello world")},
		{"long text", bytes.Repeat([]byte("a"), 1000)},
		{"binary data", []byte{0x00, 0xFF, 0x80, 0x7F}},
		{"unicode", []byte("你好世界🔐")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := mode.Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}

			// 密文应包含 nonce + 密文 + 标签
			expectedMinLen := AesGcmNonceSize + AesGcmTagSize
			if len(ciphertext) < expectedMinLen {
				t.Fatalf("ciphertext too short: %d < %d", len(ciphertext), expectedMinLen)
			}

			decrypted, err := mode.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, tc.plaintext) {
				t.Fatalf("decrypted data mismatch:\nwant: %v\ngot:  %v", tc.plaintext, decrypted)
			}
		})
	}
}

func TestAESGCMMode_EncryptProducesDifferentCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, err := NewAESGCMMode(key, nil)
	if err != nil {
		t.Fatalf("failed to create mode: %v", err)
	}

	plaintext := []byte("same plaintext")

	// 加密 10 次，每次应产生不同的密文
	ciphertexts := make([][]byte, 10)
	for i := 0; i < 10; i++ {
		ct, err := mode.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("encrypt %d failed: %v", i, err)
		}
		ciphertexts[i] = ct
	}

	// 检查所有密文对是否不同
	for i := 0; i < len(ciphertexts); i++ {
		for j := i + 1; j < len(ciphertexts); j++ {
			if bytes.Equal(ciphertexts[i], ciphertexts[j]) {
				t.Fatalf("ciphertexts %d and %d are identical", i, j)
			}
		}
	}
}

func TestAESGCMMode_CiphertextFormat(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, err := NewAESGCMMode(key, nil)
	if err != nil {
		t.Fatalf("failed to create mode: %v", err)
	}

	plaintext := []byte("test")
	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 密文格式：[12B nonce][密文][16B 标签]
	// 对于 4 字节的明文，密文长度应为 12 + 4 + 16 = 32
	expectedLen := AesGcmNonceSize + len(plaintext) + AesGcmTagSize
	if len(ciphertext) != expectedLen {
		t.Fatalf("expected ciphertext length %d, got %d", expectedLen, len(ciphertext))
	}
}

// ---------------------------------------------------------------------------
// Decrypt 错误场景测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_Decrypt_TooShort(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, err := NewAESGCMMode(key, nil)
	if err != nil {
		t.Fatalf("failed to create mode: %v", err)
	}

	testCases := []int{0, 1, 10, 27} // 小于 28 字节

	for _, len := range testCases {
		t.Run(string(rune(len)), func(t *testing.T) {
			ciphertext := make([]byte, len)
			_, err := mode.Decrypt(ciphertext)
			if err == nil {
				t.Fatalf("expected error for ciphertext length %d, got nil", len)
			}
		})
	}
}

func TestAESGCMMode_Decrypt_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	mode1, _ := NewAESGCMMode(key1, nil)
	mode2, _ := NewAESGCMMode(key2, nil)

	plaintext := []byte("secret data")
	ciphertext, err := mode1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 使用错误的密钥解密应失败
	_, err = mode2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestAESGCMMode_Decrypt_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, _ := NewAESGCMMode(key, nil)

	plaintext := []byte("test data")
	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 篡改密文（修改 nonce 后的第一个字节）
	if len(ciphertext) > AesGcmNonceSize {
		ciphertext[AesGcmNonceSize] ^= 0xFF
	}

	_, err = mode.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decryption to fail with tampered ciphertext")
	}
}

func TestAESGCMMode_Decrypt_TamperedNonce(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, _ := NewAESGCMMode(key, nil)

	plaintext := []byte("test data")
	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 篡改 nonce
	ciphertext[0] ^= 0xFF

	_, err = mode.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decryption to fail with tampered nonce")
	}
}

// ---------------------------------------------------------------------------
// Additional Data (AAD) 测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_WithAdditionalData(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	aad := []byte("additional data")

	mode, _ := NewAESGCMMode(key, aad)

	plaintext := []byte("test data")
	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	decrypted, err := mode.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data mismatch")
	}
}

func TestAESGCMMode_AdditionalDataMismatch(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	// 使用不同的 AAD 创建两个模式
	mode1, _ := NewAESGCMMode(key, []byte("aad1"))
	mode2, _ := NewAESGCMMode(key, []byte("aad2"))

	plaintext := []byte("test data")
	ciphertext, err := mode1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 使用不同的 AAD 解密应失败
	_, err = mode2.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected decryption to fail with different AAD")
	}
}

func TestAESGCMMode_AdditionalDataNilVsEmpty(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode1, _ := NewAESGCMMode(key, nil)
	mode2, _ := NewAESGCMMode(key, []byte{})

	plaintext := []byte("test data")
	ciphertext, err := mode1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// nil 和空切片应该等价
	decrypted, err := mode2.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data mismatch")
	}
}

// ---------------------------------------------------------------------------
// 不同密钥长度测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_DifferentKeyLengths(t *testing.T) {
	keyLengths := []int{16, 24, 32}

	for _, keyLen := range keyLengths {
		t.Run(string(rune(keyLen)), func(t *testing.T) {
			key := make([]byte, keyLen)
			rand.Read(key)

			mode, err := NewAESGCMMode(key, nil)
			if err != nil {
				t.Fatalf("failed to create mode with key length %d: %v", keyLen, err)
			}

			plaintext := []byte("test data")
			ciphertext, err := mode.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("encrypt failed: %v", err)
			}

			decrypted, err := mode.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, plaintext) {
				t.Fatal("decrypted data mismatch")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 边界条件测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_EmptyPlaintext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, _ := NewAESGCMMode(key, nil)

	// 加密空明文
	ciphertext, err := mode.Encrypt([]byte{})
	if err != nil {
		t.Fatalf("encrypt empty plaintext failed: %v", err)
	}

	// 密文应只包含 nonce + 标签
	expectedLen := AesGcmNonceSize + AesGcmTagSize
	if len(ciphertext) != expectedLen {
		t.Fatalf("expected length %d, got %d", expectedLen, len(ciphertext))
	}

	// 解密应返回空
	decrypted, err := mode.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Fatalf("expected empty decrypted, got length %d", len(decrypted))
	}
}

func TestAESGCMMode_LargePlaintext(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, _ := NewAESGCMMode(key, nil)

	// 测试 1MB 明文
	plaintext := make([]byte, 1024*1024)
	rand.Read(plaintext)

	ciphertext, err := mode.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt large plaintext failed: %v", err)
	}

	decrypted, err := mode.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted large plaintext mismatch")
	}
}

// ---------------------------------------------------------------------------
// 并发安全性测试
// ---------------------------------------------------------------------------

func TestAESGCMMode_ConcurrentEncrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	mode, _ := NewAESGCMMode(key, nil)

	const numGoroutines = 10
	results := make([][]byte, numGoroutines)
	errors := make([]error, numGoroutines)

	plaintext := []byte("concurrent test")

	done := make(chan bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			ct, err := mode.Encrypt(plaintext)
			results[idx] = ct
			errors[idx] = err
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 检查错误
	for i, err := range errors {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	// 所有密文应该不同（因为 nonce 随机）
	for i := 0; i < numGoroutines; i++ {
		for j := i + 1; j < numGoroutines; j++ {
			if bytes.Equal(results[i], results[j]) {
				t.Fatalf("ciphertexts %d and %d are identical", i, j)
			}
		}
	}

	// 所有密文都应该能正确解密
	for i, ct := range results {
		decrypted, err := mode.Decrypt(ct)
		if err != nil {
			t.Fatalf("decrypt ciphertext %d failed: %v", i, err)
		}
		if !bytes.Equal(decrypted, plaintext) {
			t.Fatalf("ciphertext %d decrypted mismatch", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 集成测试：完整加密解密流程
// ---------------------------------------------------------------------------

func TestAESGCMMode_Integration(t *testing.T) {
	// 模拟真实使用场景
	key := make([]byte, 32)
	rand.Read(key)

	aad := []byte("user-id:12345")
	mode, err := NewAESGCMMode(key, aad)
	if err != nil {
		t.Fatalf("failed to create mode: %v", err)
	}

	// 加密敏感数据
	sensitiveData := []byte(`{"password":"secret123","ssn":"123-45-6789"}`)

	ciphertext, err := mode.Encrypt(sensitiveData)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	// 验证密文与明文不同
	if bytes.Equal(ciphertext, sensitiveData) {
		t.Fatal("ciphertext should not equal plaintext")
	}

	// 解密
	decrypted, err := mode.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	// 验证解密结果
	if !bytes.Equal(decrypted, sensitiveData) {
		t.Fatal("decrypted data mismatch")
	}
}

package utils

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestFileExtension 测试 FileExtension 函数
func TestFileExtension(t *testing.T) {
	Convey("Test FileExtension function", t, func() {
		testCases := []struct {
			urlPath string
			ext     string
		}{
			{urlPath: "/static/js/app.js", ext: "js"},
			{urlPath: "/docs/a.txt", ext: "txt"},
			{urlPath: "/index.html", ext: "html"},
			{urlPath: "/app.min.js", ext: "js"},
			{urlPath: "/download/archive.tar.gz", ext: "gz"},
			{urlPath: "/UPLOAD/PHOTO.JPG", ext: "jpg"},
			{urlPath: "/api/users", ext: ""},
			{urlPath: "/api/users/", ext: ""},
			{urlPath: "/", ext: ""},
			{urlPath: "/.gitignore", ext: ""},
		}
		for _, tc := range testCases {
			Convey(fmt.Sprintf("path %q -> ext %q", tc.urlPath, tc.ext), func() {
				So(FileExtension(tc.urlPath), ShouldEqual, tc.ext)
			})
		}
	})
}

// TestCalculateFileMD5 测试 CalculateFileMD5 函数
func TestCalculateFileMD5(t *testing.T) {
	Convey("Test CalculateFileMD5 function", t, func() {
		// 创建临时目录用于测试
		tempDir := t.TempDir()

		Convey("Should calculate correct MD5 for text file", func() {
			// 创建测试文件
			testContent := "Hello, World! This is a test file for MD5 calculation."
			testFilePath := filepath.Join(tempDir, "test.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			// 计算期望的 MD5
			expectedHash := md5.Sum([]byte(testContent))

			// 调用被测函数
			actualHash, err := CalculateFileMD5(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, md5.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct MD5 for binary file", func() {
			// 创建二进制测试文件
			binaryData := make([]byte, 1024)
			for i := range binaryData {
				binaryData[i] = byte(i % 256)
			}
			
			testFilePath := filepath.Join(tempDir, "test.bin")
			err := os.WriteFile(testFilePath, binaryData, 0644)
			So(err, ShouldBeNil)

			// 计算期望的 MD5
			expectedHash := md5.Sum(binaryData)

			// 调用被测函数
			actualHash, err := CalculateFileMD5(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, md5.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct MD5 for empty file", func() {
			// 创建空文件
			testFilePath := filepath.Join(tempDir, "empty.txt")
			file, err := os.Create(testFilePath)
			So(err, ShouldBeNil)
			file.Close()

			// 计算期望的 MD5（空数据的 MD5）
			expectedHash := md5.Sum([]byte{})

			// 调用被测函数
			actualHash, err := CalculateFileMD5(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, md5.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct MD5 for large file", func() {
			// 创建大文件（1MB）
			testFilePath := filepath.Join(tempDir, "large.bin")
			file, err := os.Create(testFilePath)
			So(err, ShouldBeNil)
			defer file.Close()

			// 写入 1MB 数据
			data := make([]byte, 1024*1024)
			for i := range data {
				data[i] = byte(i % 256)
			}
			
			_, err = file.Write(data)
			So(err, ShouldBeNil)
			file.Close()

			// 重新打开文件计算期望的 MD5
			file, err = os.Open(testFilePath)
			So(err, ShouldBeNil)
			defer file.Close()

			hash := md5.New()
			_, err = io.Copy(hash, file)
			So(err, ShouldBeNil)
			expectedHash := hash.Sum(nil)

			// 调用被测函数
			actualHash, err := CalculateFileMD5(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, md5.Size)
			So(actualHash, ShouldResemble, expectedHash)
		})

		Convey("Should return error for non-existent file", func() {
			nonExistentPath := filepath.Join(tempDir, "non_existent_file.txt")
			hash, err := CalculateFileMD5(nonExistentPath)
			So(err, ShouldNotBeNil)
			So(hash, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "no such file")
		})

		Convey("Should return error for directory path", func() {
			hash, err := CalculateFileMD5(tempDir)
			So(err, ShouldNotBeNil)
			So(hash, ShouldBeNil)
		})

		Convey("Should return consistent results for same file", func() {
			// 创建测试文件
			testContent := "Consistency test content"
			testFilePath := filepath.Join(tempDir, "consistent.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			// 多次调用应该得到相同的结果
			hash1, err1 := CalculateFileMD5(testFilePath)
			hash2, err2 := CalculateFileMD5(testFilePath)
			hash3, err3 := CalculateFileMD5(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(err3, ShouldBeNil)
			So(hash1, ShouldResemble, hash2)
			So(hash2, ShouldResemble, hash3)
			So(hash1, ShouldResemble, hash3)
		})
	})
}

// TestCalculateFileSHA256 测试 CalculateFileSHA256 函数
func TestCalculateFileSHA256(t *testing.T) {
	Convey("Test CalculateFileSHA256 function", t, func() {
		// 创建临时目录用于测试
		tempDir := t.TempDir()

		Convey("Should calculate correct SHA-256 for text file", func() {
			// 创建测试文件
			testContent := "Hello, World! This is a test file for SHA-256 calculation."
			testFilePath := filepath.Join(tempDir, "test.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			// 计算期望的 SHA-256
			expectedHash := sha256.Sum256([]byte(testContent))

			// 调用被测函数
			actualHash, err := CalculateFileSHA256(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, sha256.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct SHA-256 for binary file", func() {
			// 创建二进制测试文件
			binaryData := make([]byte, 1024)
			for i := range binaryData {
				binaryData[i] = byte(i % 256)
			}
			
			testFilePath := filepath.Join(tempDir, "test.bin")
			err := os.WriteFile(testFilePath, binaryData, 0644)
			So(err, ShouldBeNil)

			// 计算期望的 SHA-256
			expectedHash := sha256.Sum256(binaryData)

			// 调用被测函数
			actualHash, err := CalculateFileSHA256(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, sha256.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct SHA-256 for empty file", func() {
			// 创建空文件
			testFilePath := filepath.Join(tempDir, "empty.txt")
			file, err := os.Create(testFilePath)
			So(err, ShouldBeNil)
			file.Close()

			// 计算期望的 SHA-256（空数据的 SHA-256）
			expectedHash := sha256.Sum256([]byte{})

			// 调用被测函数
			actualHash, err := CalculateFileSHA256(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, sha256.Size)
			So(actualHash, ShouldResemble, expectedHash[:])
		})

		Convey("Should calculate correct SHA-256 for large file", func() {
			// 创建大文件（2MB）
			testFilePath := filepath.Join(tempDir, "large.bin")
			file, err := os.Create(testFilePath)
			So(err, ShouldBeNil)
			defer file.Close()

			// 写入 2MB 数据
			data := make([]byte, 2*1024*1024)
			for i := range data {
				data[i] = byte(i % 256)
			}
			
			_, err = file.Write(data)
			So(err, ShouldBeNil)
			file.Close()

			// 重新打开文件计算期望的 SHA-256
			file, err = os.Open(testFilePath)
			So(err, ShouldBeNil)
			defer file.Close()

			hash := sha256.New()
			_, err = io.Copy(hash, file)
			So(err, ShouldBeNil)
			expectedHash := hash.Sum(nil)

			// 调用被测函数
			actualHash, err := CalculateFileSHA256(testFilePath)
			So(err, ShouldBeNil)
			So(actualHash, ShouldNotBeNil)
			So(len(actualHash), ShouldEqual, sha256.Size)
			So(actualHash, ShouldResemble, expectedHash)
		})

		Convey("Should return error for non-existent file", func() {
			nonExistentPath := filepath.Join(tempDir, "non_existent_file.txt")
			hash, err := CalculateFileSHA256(nonExistentPath)
			So(err, ShouldNotBeNil)
			So(hash, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "no such file")
		})

		Convey("Should return error for directory path", func() {
			hash, err := CalculateFileSHA256(tempDir)
			So(err, ShouldNotBeNil)
			So(hash, ShouldBeNil)
		})

		Convey("Should return consistent results for same file", func() {
			// 创建测试文件
			testContent := "Consistency test content for SHA-256"
			testFilePath := filepath.Join(tempDir, "consistent.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			// 多次调用应该得到相同的结果
			hash1, err1 := CalculateFileSHA256(testFilePath)
			hash2, err2 := CalculateFileSHA256(testFilePath)
			hash3, err3 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(err3, ShouldBeNil)
			So(hash1, ShouldResemble, hash2)
			So(hash2, ShouldResemble, hash3)
			So(hash1, ShouldResemble, hash3)
		})

		Convey("Should produce different hash from MD5 for same content", func() {
			// 创建测试文件
			testContent := "Same content, different hash algorithms"
			testFilePath := filepath.Join(tempDir, "comparison.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			// 计算 MD5 和 SHA-256
			md5Hash, err1 := CalculateFileMD5(testFilePath)
			sha256Hash, err2 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(md5Hash, ShouldNotBeNil)
			So(sha256Hash, ShouldNotBeNil)
			
			// MD5 是 16 字节，SHA-256 是 32 字节
			So(len(md5Hash), ShouldEqual, md5.Size)
			So(len(sha256Hash), ShouldEqual, sha256.Size)
			
			// 哈希值应该不同
			So(md5Hash, ShouldNotResemble, sha256Hash[:md5.Size])
		})
	})
}

// TestFileHashFunctionsIntegration 测试文件哈希函数的集成测试
func TestFileHashFunctionsIntegration(t *testing.T) {
	Convey("Test file hash functions integration", t, func() {
		tempDir := t.TempDir()

		Convey("Should handle file with special characters in name", func() {
			// 测试文件名包含特殊字符
			testContent := "File with special name"
			testFilePath := filepath.Join(tempDir, "test file with spaces.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			md5Hash, err1 := CalculateFileMD5(testFilePath)
			sha256Hash, err2 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(md5Hash, ShouldNotBeNil)
			So(sha256Hash, ShouldNotBeNil)
		})

		Convey("Should handle file with unicode characters", func() {
			// 测试文件内容包含 Unicode 字符
			testContent := "Hello 世界! 这是一个测试文件。"
			testFilePath := filepath.Join(tempDir, "unicode_test.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			md5Hash, err1 := CalculateFileMD5(testFilePath)
			sha256Hash, err2 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(md5Hash, ShouldNotBeNil)
			So(sha256Hash, ShouldNotBeNil)
		})

		Convey("Should verify known test vectors", func() {
			// 测试已知的哈希值（空字符串）
			testContent := ""
			testFilePath := filepath.Join(tempDir, "empty_string.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0644)
			So(err, ShouldBeNil)

			md5Hash, err1 := CalculateFileMD5(testFilePath)
			sha256Hash, err2 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)

			// 空字符串的 MD5: d41d8cd98f00b204e9800998ecf8427e
			expectedMD5 := []byte{0xd4, 0x1d, 0x8c, 0xd9, 0x8f, 0x00, 0xb2, 0x04, 0xe9, 0x80, 0x09, 0x98, 0xec, 0xf8, 0x42, 0x7e}
			So(md5Hash, ShouldResemble, expectedMD5)

			// 空字符串的 SHA-256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
			expectedSHA256 := []byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}
			So(sha256Hash, ShouldResemble, expectedSHA256)
		})

		Convey("Should handle file permissions correctly", func() {
			// 测试文件权限
			testContent := "Permission test"
			testFilePath := filepath.Join(tempDir, "permission_test.txt")
			
			err := os.WriteFile(testFilePath, []byte(testContent), 0400) // 只读权限
			So(err, ShouldBeNil)

			// 应该能够读取文件计算哈希
			md5Hash, err1 := CalculateFileMD5(testFilePath)
			sha256Hash, err2 := CalculateFileSHA256(testFilePath)

			So(err1, ShouldBeNil)
			So(err2, ShouldBeNil)
			So(md5Hash, ShouldNotBeNil)
			So(sha256Hash, ShouldNotBeNil)
		})
	})
}

// BenchmarkCalculateFileMD5 基准测试 CalculateFileMD5 函数
func BenchmarkCalculateFileMD5(b *testing.B) {
	// 创建测试文件
	tempDir := b.TempDir()
	testFilePath := filepath.Join(tempDir, "benchmark.txt")
	
	// 创建 1MB 测试数据
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	err := os.WriteFile(testFilePath, data, 0644)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := CalculateFileMD5(testFilePath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCalculateFileSHA256 基准测试 CalculateFileSHA256 函数
func BenchmarkCalculateFileSHA256(b *testing.B) {
	// 创建测试文件
	tempDir := b.TempDir()
	testFilePath := filepath.Join(tempDir, "benchmark.txt")
	
	// 创建 1MB 测试数据
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	err := os.WriteFile(testFilePath, data, 0644)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := CalculateFileSHA256(testFilePath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ExampleCalculateFileMD5 CalculateFileMD5 函数的使用示例
func ExampleCalculateFileMD5() {
	// 在实际使用中，您需要提供一个真实的文件路径
	// 这里为了示例，我们创建一个临时文件
	tempDir, err := os.MkdirTemp("", "example")
	if err != nil {
		fmt.Printf("Error creating temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir)

	testFilePath := filepath.Join(tempDir, "example.txt")
	content := "This is an example file for MD5 calculation."
	
	if err := os.WriteFile(testFilePath, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	hash, err := CalculateFileMD5(testFilePath)
	if err != nil {
		fmt.Printf("Error calculating MD5: %v\n", err)
		return
	}

	fmt.Printf("MD5 hash: %x\n", hash)
	// 输出: MD5 hash: [实际的 MD5 哈希值]
}

// ExampleCalculateFileSHA256 CalculateFileSHA256 函数的使用示例
func ExampleCalculateFileSHA256() {
	// 在实际使用中，您需要提供一个真实的文件路径
	// 这里为了示例，我们创建一个临时文件
	tempDir, err := os.MkdirTemp("", "example")
	if err != nil {
		fmt.Printf("Error creating temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(tempDir)

	testFilePath := filepath.Join(tempDir, "example.txt")
	content := "This is an example file for SHA-256 calculation."
	
	if err := os.WriteFile(testFilePath, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	hash, err := CalculateFileSHA256(testFilePath)
	if err != nil {
		fmt.Printf("Error calculating SHA-256: %v\n", err)
		return
	}

	fmt.Printf("SHA-256 hash: %x\n", hash)
	// 输出: SHA-256 hash: [实际的 SHA-256 哈希值]
}
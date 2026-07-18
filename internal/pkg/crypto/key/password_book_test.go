package key

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// PasswordBook 基本方法测试
// ---------------------------------------------------------------------------

func TestPasswordBook_Bytes(t *testing.T) {
	data := []byte("test data for password book")
	pb := &PasswordBook{data: data}

	got := pb.Bytes()
	if string(got) != string(data) {
		t.Fatalf("Bytes() returned %q, want %q", got, data)
	}
}

func TestPasswordBook_Len(t *testing.T) {
	data := make([]byte, 1024)
	pb := &PasswordBook{data: data}

	if got := pb.Len(); got != 1024 {
		t.Fatalf("Len() returned %d, want 1024", got)
	}
}

func TestPasswordBook_EmptyData(t *testing.T) {
	pb := &PasswordBook{data: []byte{}}
	if pb.Len() != 0 {
		t.Fatalf("Len() returned %d, want 0", pb.Len())
	}
	if len(pb.Bytes()) != 0 {
		t.Fatalf("Bytes() returned non-empty slice")
	}
}

// ---------------------------------------------------------------------------
// LoadPasswordBookFromFile 测试
// ---------------------------------------------------------------------------

func TestLoadPasswordBookFromFile_ValidFile(t *testing.T) {
	// 创建临时文件，大小恰好为 PASSWORD_BOOK_SIZE
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_password_book.bin")

	data := make([]byte, PASSWORD_BOOK_SIZE)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	pb, err := LoadPasswordBookFromFile(path)
	if err != nil {
		t.Fatalf("LoadPasswordBookFromFile() returned error: %v", err)
	}
	if pb.Len() != PASSWORD_BOOK_SIZE {
		t.Fatalf("Len() returned %d, want %d", pb.Len(), PASSWORD_BOOK_SIZE)
	}
}

func TestLoadPasswordBookFromFile_FileNotExist(t *testing.T) {
	_, err := LoadPasswordBookFromFile("/nonexistent/path/password_book.bin")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoadPasswordBookFromFile_SizeMismatch_TooSmall(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "small_password_book.bin")

	data := make([]byte, 100) // 远小于 PASSWORD_BOOK_SIZE
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := LoadPasswordBookFromFile(path)
	if err == nil {
		t.Fatal("expected error for size mismatch, got nil")
	}
}

func TestLoadPasswordBookFromFile_SizeMismatch_TooLarge(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "large_password_book.bin")

	data := make([]byte, PASSWORD_BOOK_SIZE+1)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := LoadPasswordBookFromFile(path)
	if err == nil {
		t.Fatal("expected error for size mismatch, got nil")
	}
}

func TestLoadPasswordBookFromFile_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty_password_book.bin")

	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := LoadPasswordBookFromFile(path)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

// ---------------------------------------------------------------------------
// LoadPasswordBook 测试
// ---------------------------------------------------------------------------

func TestLoadPasswordBook_EmptyPath_FallsBackToEmbedded(t *testing.T) {
	pb, err := LoadPasswordBook("")
	if err != nil {
		t.Fatalf("LoadPasswordBook('') returned error: %v", err)
	}
	if pb.Len() != PASSWORD_BOOK_SIZE {
		t.Fatalf("Len() returned %d, want %d", pb.Len(), PASSWORD_BOOK_SIZE)
	}
}

func TestLoadPasswordBook_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "custom_password_book.bin")

	data := make([]byte, PASSWORD_BOOK_SIZE)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	pb, err := LoadPasswordBook(path)
	if err != nil {
		t.Fatalf("LoadPasswordBook() returned error: %v", err)
	}
	if pb.Len() != PASSWORD_BOOK_SIZE {
		t.Fatalf("Len() returned %d, want %d", pb.Len(), PASSWORD_BOOK_SIZE)
	}
}

func TestLoadPasswordBook_NonexistentFile_FallsBackToEmbedded(t *testing.T) {
	// 文件不存在时，应回退到内置密码本
	pb, err := LoadPasswordBook("/nonexistent/path/password_book.bin")
	if err != nil {
		t.Fatalf("LoadPasswordBook() returned error: %v", err)
	}
	if pb.Len() != PASSWORD_BOOK_SIZE {
		t.Fatalf("Len() returned %d, want %d", pb.Len(), PASSWORD_BOOK_SIZE)
	}
}

func TestLoadPasswordBook_InvalidSizeFile_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid_size.bin")

	// 文件大小不正确（存在但大小不对）
	data := make([]byte, 100)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := LoadPasswordBook(path)
	if err == nil {
		t.Fatal("expected error for file with wrong size, got nil")
	}
}

// ---------------------------------------------------------------------------
// loadEmbeddedPasswordBook 测试
// ---------------------------------------------------------------------------

func TestLoadEmbeddedPasswordBook(t *testing.T) {
	pb, err := loadEmbeddedPasswordBook()
	if err != nil {
		t.Fatalf("loadEmbeddedPasswordBook() returned error: %v", err)
	}
	if pb.Len() != PASSWORD_BOOK_SIZE {
		t.Fatalf("Len() returned %d, want %d", pb.Len(), PASSWORD_BOOK_SIZE)
	}
}

// ---------------------------------------------------------------------------
// PASSWORD_BOOK_SIZE 常量测试
// ---------------------------------------------------------------------------

func TestPasswordBookSize(t *testing.T) {
	if PASSWORD_BOOK_SIZE != 65536 {
		t.Fatalf("PASSWORD_BOOK_SIZE = %d, want 65536", PASSWORD_BOOK_SIZE)
	}
}

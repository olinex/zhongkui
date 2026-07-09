package crypto

import (
	_ "embed"
	"fmt"
	"os"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

//go:embed default_password_book.bin
var defaultPasswordBookData []byte

type PasswordBook struct {
	data []byte
}

func (p *PasswordBook) Bytes() []byte {
	return p.data
}

func (p *PasswordBook) Len() int {
	return len(p.data)
}

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

func LoadPasswordBook(path string) (*PasswordBook, error) {
	if path != "" {
		pb, err := LoadPasswordBookFromFile(path)
		if err == nil {
			return pb, nil
		}
		// If the file doesn't exist on disk, fall through to embedded
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

func loadEmbeddedPasswordBook() (*PasswordBook, error) {
	if len(defaultPasswordBookData) != PASSWORD_BOOK_SIZE {
		return nil, errors.Newf("embedded password book size mismatch: expected %d bytes, got %d bytes", PASSWORD_BOOK_SIZE, len(defaultPasswordBookData))
	}
	return &PasswordBook{data: defaultPasswordBookData}, nil
}

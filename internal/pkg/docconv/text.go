package docconv

import (
	"io"
	"strings"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// convertPlainText 从 ReaderAt 读全量文本，去 UTF-8 BOM 并清洗非法 UTF-8 字节后原样返回；
// markdown 与 txt 均不做结构改写。
func convertPlainText(r io.ReaderAt, size int64) (string, error) {
	data, err := io.ReadAll(io.NewSectionReader(r, 0, size))
	if err != nil {
		return "", errors.WithStack(err)
	}
	text := strings.TrimPrefix(string(data), "\uFEFF")
	return strings.ToValidUTF8(text, "\uFFFD"), nil
}

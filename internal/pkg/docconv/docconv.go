// Package docconv 提供通用文档转文本能力：把 PDF / DOCX / Markdown / TXT 文件解析为
// Markdown 文本，供知识库（RAG）导入后复用 MarkdownChunker 的结构感知切片。本包只做
// 「字节 → Markdown 文本」的无状态转换，不含任何业务逻辑，也不依赖 pkg 之外的模块。
//
// Package docconv provides generic document-to-text conversion: it parses PDF / DOCX /
// Markdown / TXT files into Markdown text so that knowledge-base (RAG) imports can reuse the
// structure-aware MarkdownChunker. The package only performs stateless "bytes -> Markdown
// text" conversion, holds no business logic and depends on no module outside internal/pkg.
package docconv

import (
	// stdlib
	"context"
	"io"
	"path/filepath"
	"strings"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Format 是文档格式的取值类型，其字符串取值即落库值。
// Format is the value type of a document format; its string value is what gets persisted.
type Format string

// 文档格式常量。取值字符串即数据库存储值，与 documentFormat 枚举类的 value 一致。
// Document format constants. The value string is the persisted database value, matching the
// value of the documentFormat enum class.
const (
	// FormatMarkdown Markdown 文档（落库值 markdown）
	// FormatMarkdown a Markdown document (persisted value markdown)
	FormatMarkdown Format = "markdown"
	// FormatTxt 纯文本文档（落库值 txt）
	// FormatTxt a plain text document (persisted value txt)
	FormatTxt Format = "txt"
	// FormatPDF PDF 文档（落库值 pdf）
	// FormatPDF a PDF document (persisted value pdf)
	FormatPDF Format = "pdf"
	// FormatDocx Word DOCX 文档（落库值 docx）
	// FormatDocx a Word DOCX document (persisted value docx)
	FormatDocx Format = "docx"
)

// Detect 按文件名扩展名（大小写不敏感）识别文档格式：.md/.markdown → markdown、.txt → txt、
// .pdf → pdf、.docx → docx；空文件名、无扩展名或其余扩展名返回英文错误。
// Detect identifies the document format from the filename extension (case-insensitively):
// .md/.markdown -> markdown, .txt -> txt, .pdf -> pdf, .docx -> docx; an empty filename, a
// missing extension or any other extension yields an English error.
func Detect(filename string) (Format, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown":
		return FormatMarkdown, nil
	case ".txt":
		return FormatTxt, nil
	case ".pdf":
		return FormatPDF, nil
	case ".docx":
		return FormatDocx, nil
	default:
		return "", errors.Newf("docconv: unsupported document format %q", ext)
	}
}

// Convert 把 r 中 size 字节的文档按 format 转换为 Markdown 文本：markdown/txt 原样返回（去 BOM、
// 清洗非法 UTF-8），docx 解析 word/document.xml 转 ATX 标题与 GFM 管道表，pdf 逐页抽取文本。
// Convert turns the size bytes readable from r into Markdown text according to format:
// markdown/txt are returned as-is (BOM stripped, invalid UTF-8 sanitized), docx parses
// word/document.xml into ATX headings and GFM pipe tables, and pdf extracts text page by page.
func Convert(ctx context.Context, format Format, r io.ReaderAt, size int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", errors.WithStack(err)
	}
	switch format {
	case FormatMarkdown, FormatTxt:
		return convertPlainText(r, size)
	case FormatDocx:
		return convertDocx(r, size)
	case FormatPDF:
		return convertPDF(r, size)
	default:
		return "", errors.Newf("docconv: unsupported document format %q", format)
	}
}

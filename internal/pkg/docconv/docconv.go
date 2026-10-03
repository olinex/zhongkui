// Package docconv 提供通用文档转文本能力：把 PDF / DOCX / Markdown / TXT 文件解析为
// Markdown 文本，供知识库（RAG）导入后复用 MarkdownChunker 的结构感知切片。本包只做
// 「字节 → Markdown 文本」的无状态转换，不含任何业务逻辑，也不依赖 pkg 之外的模块。
package docconv

import (
	// 标准库
	"context"
	"io"
	"path/filepath"
	"strings"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Format 是文档格式的取值类型，其字符串取值即落库值。
type Format string

// 文档格式常量。取值字符串即数据库存储值，与 documentFormat 枚举类的 value 一致。
const (
	// FormatMarkdown Markdown 文档（落库值 markdown）
	FormatMarkdown Format = "markdown"
	// FormatTxt 纯文本文档（落库值 txt）
	FormatTxt Format = "txt"
	// FormatPDF PDF 文档（落库值 pdf）
	FormatPDF Format = "pdf"
	// FormatDocx Word DOCX 文档（落库值 docx）
	FormatDocx Format = "docx"
	// FormatGenerated 由代码注释自动生成的文档（叙述页等）专用分区（落库值 generated）。
	// 该格式非用户导入格式，与人工导入文档相互隔离：同步生成文档时只操作该分区的文档，
	// 绝不触碰其余格式的文档。
	FormatGenerated Format = "generated"
)

// Detect 按文件名扩展名（大小写不敏感）识别文档格式：.md/.markdown → markdown、.txt → txt、
// .pdf → pdf、.docx → docx；空文件名、无扩展名或其余扩展名返回英文错误。
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

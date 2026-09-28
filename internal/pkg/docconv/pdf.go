// 本文件实现 PDF 转 Markdown：经纯 Go 解析库 github.com/ledongthuc/pdf 逐页抽取文本，单页文档
// 直接输出该页文本，多页文档每页以 `## Page N` 标题起首，页内按空行分段。
//
// This file implements PDF-to-Markdown conversion: it extracts text page by page via the pure-Go
// library github.com/ledongthuc/pdf. A single-page document outputs that page's text directly,
// while a multi-page document prefixes every page with a `## Page N` heading and separates
// paragraphs inside a page with a blank line.
package docconv

import (
	// stdlib
	"fmt"
	"io"
	"strings"

	// third-party
	"github.com/ledongthuc/pdf"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// convertPDF 逐页抽取 PDF 文本并拼装为 Markdown：单页直接返回该页文本；多页每页以 `## Page N`
// 标题起首（N 从 1 起，标题后空一行）；空文本页跳过标题但保留页码序号。
// convertPDF extracts PDF text page by page and assembles Markdown: a single page returns its
// text directly; for multiple pages every page is prefixed with a `## Page N` heading (N starts
// at 1, followed by a blank line); a page with empty text skips its heading while the page
// numbering stays aligned.
func convertPDF(r io.ReaderAt, size int64) (string, error) {
	reader, err := pdf.NewReader(r, size)
	if err != nil {
		return "", errors.WithStack(err)
	}
	pages := reader.NumPage()
	if pages <= 0 {
		return "", nil
	}
	texts := make([]string, 0, pages)
	for i := 1; i <= pages; i++ {
		text, err := reader.Page(i).GetPlainText(nil)
		if err != nil {
			return "", errors.WithStack(err)
		}
		texts = append(texts, normalizePDFPageText(text))
	}
	if pages == 1 {
		return texts[0], nil
	}
	blocks := make([]string, 0, pages)
	for i, text := range texts {
		if text == "" {
			continue
		}
		blocks = append(blocks, fmt.Sprintf("## Page %d\n\n%s", i+1, text))
	}
	return strings.Join(blocks, "\n\n"), nil
}

// normalizePDFPageText 规整单页抽取文本：去掉行尾空白、折叠连续空行为单个空行、去首尾空行，
// 使页内以空行分段，便于 MarkdownChunker 识别段落边界。
// normalizePDFPageText normalizes the extracted text of one page: it strips trailing line
// whitespace, collapses consecutive blank lines into one and trims leading/trailing blank lines,
// so paragraphs inside the page are separated by a single blank line for the MarkdownChunker to
// recognize paragraph boundaries.
func normalizePDFPageText(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if len(out) == 0 || out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

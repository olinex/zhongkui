// Package docconv（本文件）提供 PDF 分支的单测：在测试内手工构造最小 PDF（经典 xref 表），
// 验证单页直接输出、多页以 `## Page N` 起首。
package docconv

import (
	// 标准库
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"
)

func TestConvertPDF(t *testing.T) {
	Convey("单页 PDF 直接输出该页文本", t, func() {
		data := buildMinimalPDF([]string{"Hello Page One"})
		got, err := Convert(context.Background(), FormatPDF, bytes.NewReader(data), int64(len(data)))
		So(err, ShouldBeNil)
		So(got, ShouldContainSubstring, "Hello Page One")
		So(got, ShouldNotContainSubstring, "## Page")
	})

	Convey("多页 PDF 每页以 ## Page N 标题起首", t, func() {
		data := buildMinimalPDF([]string{"First body text", "Second body text"})
		got, err := Convert(context.Background(), FormatPDF, bytes.NewReader(data), int64(len(data)))
		So(err, ShouldBeNil)
		So(got, ShouldContainSubstring, "## Page 1")
		So(got, ShouldContainSubstring, "## Page 2")
		So(got, ShouldContainSubstring, "First body text")
		So(got, ShouldContainSubstring, "Second body text")
	})
}

// buildMinimalPDF 手工构造含 n 页的最小 PDF 字节：目录 + 页树 + 共享 Helvetica 字体 + 每页的
// 页对象与内容流，并按经典 xref 表给出精确字节偏移（页文本须为 ASCII 以保证偏移量正确）。
func buildMinimalPDF(pageTexts []string) []byte {
	n := len(pageTexts)
	total := 3 + 2*n // 对象 1..total：1 catalog、2 pages、3 font，其后每页 page+content
	var sb strings.Builder
	sb.WriteString("%PDF-1.4\n")
	offsets := make([]int, total+1)
	writeObj := func(id int, body string) {
		offsets[id] = sb.Len()
		sb.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, body))
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	kids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+2*i))
	}
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	writeObj(3, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, text := range pageTexts {
		pageID, contentID := 4+2*i, 5+2*i
		writeObj(pageID, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>",
			contentID))
		stream := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET", text)
		writeObj(contentID, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}

	xrefPos := sb.Len()
	sb.WriteString(fmt.Sprintf("xref\n0 %d\n", total+1))
	sb.WriteString("0000000000 65535 f \n")
	for id := 1; id <= total; id++ {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[id]))
	}
	sb.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", total+1, xrefPos))
	return []byte(sb.String())
}

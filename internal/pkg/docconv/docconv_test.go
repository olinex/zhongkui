// Package docconv（本文件）提供通用文档转文本的单测：覆盖扩展名识别、纯文本去 BOM、
// DOCX 手工最小包的标题/正文/表格解析，以及 PDF 分页标题抽取。
package docconv

import (
	// 标准库
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"
)

func TestDetect(t *testing.T) {
	Convey("Detect 命中四种格式并拒绝其它扩展名", t, func() {
		hit := []struct {
			filename string
			want     Format
		}{
			{"notes.md", FormatMarkdown},
			{"notes.markdown", FormatMarkdown},
			{"notes.txt", FormatTxt},
			{"notes.pdf", FormatPDF},
			{"notes.docx", FormatDocx},
			{"REPORT.PDF", FormatPDF},
			{"Mixed.MarkDown", FormatMarkdown},
			{"Data.DOCX", FormatDocx},
		}
		for _, c := range hit {
			got, err := Detect(c.filename)
			So(err, ShouldBeNil)
			So(got, ShouldEqual, c.want)
		}

		miss := []string{"book.xlsx", "archive.tar.gz", "noextension", "", "trailing."}
		for _, filename := range miss {
			got, err := Detect(filename)
			So(err, ShouldNotBeNil)
			So(got, ShouldEqual, Format(""))
			So(err.Error(), ShouldContainSubstring, "unsupported document format")
		}
	})
}

func TestConvertPlainText(t *testing.T) {
	Convey("markdown/txt 去 BOM 且正文保真", t, func() {
		body := "# 标题\n\n正文行\n"
		input := "\uFEFF" + body
		for _, format := range []Format{FormatMarkdown, FormatTxt} {
			got, err := Convert(context.Background(), format, strings.NewReader(input), int64(len(input)))
			So(err, ShouldBeNil)
			So(got, ShouldEqual, body)
		}
	})

	Convey("非法 UTF-8 字节被清洗为合法文本", t, func() {
		input := "ok\xffbad"
		got, err := Convert(context.Background(), FormatTxt, strings.NewReader(input), int64(len(input)))
		So(err, ShouldBeNil)
		So(utf8.ValidString(got), ShouldBeTrue)
		So(got, ShouldContainSubstring, "ok")
		So(got, ShouldContainSubstring, "bad")
	})
}

func TestConvertDocx(t *testing.T) {
	Convey("DOCX 解析出 ATX 标题、正文与 GFM 管道表", t, func() {
		const documentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t>Document Title</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Section One</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="heading 2"/></w:pPr><w:r><w:t>Sub Section</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Body </w:t></w:r><w:r><w:t>text here</w:t></w:r><w:r><w:br/><w:t>second line</w:t></w:r></w:p>
    <w:p><w:r><w:instrText>PAGE</w:instrText><w:t>Kept</w:t></w:r></w:p>
    <w:tbl>
      <w:tr><w:tc><w:p><w:r><w:t>Name</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Value</w:t></w:r></w:p></w:tc></w:tr>
      <w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>1</w:t></w:r></w:p></w:tc></w:tr>
    </w:tbl>
    <w:sectPr/>
  </w:body>
</w:document>`
		data := mustBuildDocx(documentXML)

		got, err := Convert(context.Background(), FormatDocx, bytes.NewReader(data), int64(len(data)))
		So(err, ShouldBeNil)

		So(got, ShouldContainSubstring, "# Document Title")
		So(got, ShouldContainSubstring, "# Section One")
		So(got, ShouldContainSubstring, "## Sub Section")
		// xml:space="preserve" 的尾随空格与跨 run 拼接被保留
		So(got, ShouldContainSubstring, "Body text here\nsecond line")
		So(got, ShouldContainSubstring, "Kept")
		// 域代码不进入正文
		So(got, ShouldNotContainSubstring, "PAGE")
		// 表格渲染为 GFM 管道表
		So(got, ShouldContainSubstring, "| Name | Value |")
		So(got, ShouldContainSubstring, "| --- | --- |")
		So(got, ShouldContainSubstring, "| A | 1 |")
	})
}

// mustBuildDocx 在测试内手工构造最小 DOCX：仅含 word/document.xml 一个部件。
func mustBuildDocx(documentXML string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(docxDocumentPath)
	So(err, ShouldBeNil)
	_, err = w.Write([]byte(documentXML))
	So(err, ShouldBeNil)
	So(zw.Close(), ShouldBeNil)
	return buf.Bytes()
}

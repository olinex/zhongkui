// 本文件实现 DOCX 转 Markdown：仅用标准库 archive/zip 读取 word/document.xml，再经
// encoding/xml 的流式 token 解析按文档顺序还原段落（标题 / 正文）与表格（GFM 管道表）。
package docconv

import (
	// 标准库
	"archive/zip"
	"encoding/xml"
	"io"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// docxDocumentPath 是 DOCX 包内承载正文的部件路径。
const docxDocumentPath = "word/document.xml"

// convertDocx 读取 DOCX 的正文部件并转换为 Markdown 文本，块之间以空行分隔。
func convertDocx(r io.ReaderAt, size int64) (string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", errors.WithStack(err)
	}
	var part *zip.File
	for _, f := range zr.File {
		if f.Name == docxDocumentPath {
			part = f
			break
		}
	}
	if part == nil {
		return "", errors.Newf("docconv: docx part %q not found", docxDocumentPath)
	}
	rc, err := part.Open()
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer rc.Close()
	blocks, err := parseDocxBody(rc)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return strings.Join(blocks, "\n\n"), nil
}

// parseDocxBody 流式解析正文，按文档顺序把每个顶层段落与表格转换为一个 Markdown 块。
func parseDocxBody(rc io.Reader) ([]string, error) {
	dec := xml.NewDecoder(rc)
	var blocks []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		// 表格整体解码（会吞掉其内的段落与嵌套表格），故不会与顶层段落重复计数。
		switch se.Name.Local {
		case "p":
			var p docxParagraph
			if err := dec.DecodeElement(&p, &se); err != nil {
				return nil, errors.WithStack(err)
			}
			if block := p.markdown(); block != "" {
				blocks = append(blocks, block)
			}
		case "tbl":
			var t docxTable
			if err := dec.DecodeElement(&t, &se); err != nil {
				return nil, errors.WithStack(err)
			}
			if block := t.markdown(); block != "" {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks, nil
}

// docxParagraph 是一个解析后的 Word 段落：样式名与按文档顺序拼装的纯文本。
type docxParagraph struct {
	style string
	text  string
}

// UnmarshalXML 流式消费段落的全部子元素：收集 w:t 文本，把 w:br/w:cr 还原为换行、w:tab 还原为
// 制表符，忽略 w:instrText 域代码；遍历整个子树可保证这些行内元素按出现顺序拼接。
func (p *docxParagraph) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return errors.WithStack(err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pStyle":
				var s docxVal
				if err := d.DecodeElement(&s, &t); err != nil {
					return errors.WithStack(err)
				}
				p.style = s.Val
			case "t":
				var s docxText
				if err := d.DecodeElement(&s, &t); err != nil {
					return errors.WithStack(err)
				}
				// xml:space="preserve" 时 chardata 原样保留前导/尾随空格
				b.WriteString(s.Value)
			case "br", "cr":
				b.WriteString("\n")
				if err := d.Skip(); err != nil {
					return errors.WithStack(err)
				}
			case "tab":
				b.WriteString("\t")
				if err := d.Skip(); err != nil {
					return errors.WithStack(err)
				}
			case "instrText":
				// 域代码（如页码域）不属正文，跳过以免脏文本
				if err := d.Skip(); err != nil {
					return errors.WithStack(err)
				}
			}
		case xml.EndElement:
			if t.Name == start.Name {
				p.text = b.String()
				return nil
			}
		}
	}
}

// markdown 把一个解析后的段落渲染为 Markdown 块：命中标题样式时输出 ATX 标题，否则输出正文
// （保留行内空白）；纯空白段落返回空串（不产生块）。
func (p docxParagraph) markdown() string {
	if strings.TrimSpace(p.text) == "" {
		return ""
	}
	if level := headingLevel(p.style); level > 0 {
		return strings.Repeat("#", level) + " " + strings.Join(strings.Fields(p.text), " ")
	}
	return p.text
}

// headingLevel 把段落样式名映射为 ATX 标题级别：Title → 1，Heading1..Heading6 → 1..6；
// 兼容空格与大小写变体（如 "heading 1"、"Heading1"）。非标题样式返回 0。
func headingLevel(style string) int {
	s := strings.ToLower(strings.TrimSpace(style))
	s = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s)
	if s == "title" {
		return 1
	}
	if rest, ok := strings.CutPrefix(s, "heading"); ok && rest != "" {
		if level, err := strconv.Atoi(rest); err == nil && level >= 1 && level <= 6 {
			return level
		}
	}
	return 0
}

// docxTable 是一个解析后的 Word 表格，按行保存每个单元格的文本。
type docxTable struct {
	rows [][]string
}

// UnmarshalXML 流式消费表格：每遇 w:tr 结束即结算一行，每遇 w:tc 解码单元格；嵌套表格整体跳过，
// 避免其中的行/单元格污染外层结构。
func (t *docxTable) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var (
		row   []string
		inRow bool
	)
	for {
		tok, err := d.Token()
		if err != nil {
			return errors.WithStack(err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tr":
				row, inRow = nil, true
			case "tc":
				cell, err := decodeTableCell(d, e)
				if err != nil {
					return errors.WithStack(err)
				}
				if inRow {
					row = append(row, cell)
				}
			case "tbl":
				if err := d.Skip(); err != nil {
					return errors.WithStack(err)
				}
			}
		case xml.EndElement:
			switch {
			case e.Name == start.Name:
				return nil
			case e.Name.Local == "tr" && inRow:
				t.rows = append(t.rows, row)
				row, inRow = nil, false
			}
		}
	}
}

// decodeTableCell 解码一个单元格：把其内各段落文本以空格连接，嵌套表格跳过。
func decodeTableCell(d *xml.Decoder, start xml.StartElement) (string, error) {
	var parts []string
	for {
		tok, err := d.Token()
		if err != nil {
			return "", errors.WithStack(err)
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "p":
				var p docxParagraph
				if err := d.DecodeElement(&p, &e); err != nil {
					return "", errors.WithStack(err)
				}
				if s := strings.TrimSpace(p.text); s != "" {
					parts = append(parts, s)
				}
			case "tbl":
				if err := d.Skip(); err != nil {
					return "", errors.WithStack(err)
				}
			}
		case xml.EndElement:
			if e.Name == start.Name {
				return strings.Join(parts, " "), nil
			}
		}
	}
}

// markdown 把表格渲染为 GFM 管道表：首行为表头，其后是分隔行，其余行为数据行；各行列数补齐到
// 最大列数。无行或零列时返回空串。
func (t docxTable) markdown() string {
	if len(t.rows) == 0 {
		return ""
	}
	width := 0
	for _, row := range t.rows {
		if len(row) > width {
			width = len(row)
		}
	}
	if width == 0 {
		return ""
	}
	var b strings.Builder
	writeRow := func(row []string) {
		b.WriteString("|")
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(row) {
				cell = escapeTableCell(row[i])
			}
			b.WriteString(" " + cell + " |")
		}
		b.WriteString("\n")
	}
	writeRow(t.rows[0])
	b.WriteString("|")
	for i := 0; i < width; i++ {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range t.rows[1:] {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

// escapeTableCell 清洗单元格文本：换行转空格、转义管道符、去首尾空白，避免破坏管道表结构。
func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.TrimSpace(s)
}

// docxVal 承载带 w:val 属性的 Word 元素（如 w:pStyle）。
type docxVal struct {
	Val string `xml:"val,attr"`
}

// docxText 承载 w:t 的字符数据（xml:space="preserve" 时原样保留空格）。
type docxText struct {
	Value string `xml:",chardata"`
}

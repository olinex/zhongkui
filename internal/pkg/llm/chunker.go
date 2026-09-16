package llm

import (
	// stdlib
	"context"
	"strconv"
	"strings"

	// third-party
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// 默认切片参数：ChunkingConfig 的 default 标签值（size=800, overlap=100），
// 当构造参数非正数时回退使用。
const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
)

// mdUnitKind 标识 markdown 解析出的原子单元类型。
// mdUnitKind identifies the kind of an atomic unit parsed from markdown.
type mdUnitKind int

const (
	// unitHeading 标题行（硬边界标记，自身不产出块正文）
	// unitHeading a heading line (a hard-boundary marker, never part of a chunk body)
	unitHeading mdUnitKind = iota
	// unitParagraph 普通段落
	// unitParagraph a plain paragraph
	unitParagraph
	// unitCodeBlock 围栏代码块
	// unitCodeBlock a fenced code block
	unitCodeBlock
	// unitTable 表格块
	// unitTable a table block
	unitTable
	// unitList 列表块
	// unitList a list block
	unitList
	// unitQuote 引用块
	// unitQuote a quote block
	unitQuote
)

// mdUnit 是行级解析 markdown 得到的不可拆散原子单元。
// mdUnit is an unsplittable atomic unit produced by line-level markdown parsing.
type mdUnit struct {
	kind mdUnitKind // 单元类型
	text string     // 单元正文（不含标题行）
	path string     // 单元所在标题路径（如 "Root / Sub"）
}

// MarkdownChunker 是 Markdown 结构感知切片器，实现 eino document.Transformer 接口：
// 输入 Document.Content 为 markdown 原文，输出切片后的 chunk 文档，每个 chunk 的
// MetaData 携带 chunkIndex、headingPath 并保留输入 MetaData。分块规则：
//   - 行级解析 ATX 标题（#..######，记录层级与标题路径）、围栏代码块（```/~~~ 成对，
//     未闭合视为延伸到文档末尾，绝不从中切开）、表格块（连续 | 行）、列表块、引用块与
//     普通段落；
//   - 标题为硬边界：命中标题即结算当前块并更新标题路径，正文不含标题重复行（标题文本
//     只存在于 headingPath）；
//   - 普通段落/列表/引用按 chunking.size 亲和目标长度合并；代码块/表格为原子单元，
//     即使整体超长也整体成一块，不硬切；
//   - 相邻块之间按 chunking.overlap 字符重叠衔接（复用上一块尾部文本）。
//
// MarkdownChunker is a Markdown structure-aware chunker implementing the eino
// document.Transformer interface: it consumes Document.Content as raw markdown and
// emits chunk documents whose MetaData carries chunkIndex and headingPath while
// preserving the input MetaData. Chunking rules:
//   - line-level parsing of ATX headings (#..###### with level and heading path),
//     fenced code blocks (paired ```/~~~, an unclosed fence extends to the end of the
//     document and is never split), table blocks (consecutive | lines), list blocks,
//     quote blocks and plain paragraphs;
//   - a heading is a hard boundary: hitting one settles the current chunk and updates
//     the heading path; the chunk body never repeats heading lines (heading text only
//     lives in headingPath);
//   - plain paragraphs/lists/quotes merge toward the chunking.size affinity target;
//     code blocks and tables are atomic units that stay whole even when oversized;
//   - adjacent chunks overlap by chunking.overlap characters (reusing the previous
//     chunk tail text).
type MarkdownChunker struct {
	size    int // 亲和目标长度（字符）
	overlap int // 相邻块重叠长度（字符）
}

// NewMarkdownChunker 构造 MarkdownChunker。size <= 0 时回退默认 800，overlap < 0 时按 0 处理。
// NewMarkdownChunker constructs a MarkdownChunker. A non-positive size falls back to the
// default 800; a negative overlap is treated as 0.
func NewMarkdownChunker(size, overlap int) *MarkdownChunker {
	if size <= 0 {
		size = defaultChunkSize
	}
	if overlap < 0 {
		overlap = 0
	}
	return &MarkdownChunker{size: size, overlap: overlap}
}

// Transform 对每个输入文档执行 markdown 结构感知切片，返回切片后的 chunk 文档列表。
// Transform applies Markdown structure-aware chunking to every input document and returns
// the resulting chunk documents.
func (c *MarkdownChunker) Transform(_ context.Context, src []*schema.Document, _ ...document.TransformerOption) ([]*schema.Document, error) {
	var out []*schema.Document
	for _, doc := range src {
		out = append(out, c.chunk(doc)...)
	}
	return out, nil
}

// chunk 将单个文档切为多个 chunk；空文档/纯空白返回空切片。
// chunk splits a single document into chunks; an empty or whitespace-only document
// yields an empty slice.
func (c *MarkdownChunker) chunk(doc *schema.Document) []*schema.Document {
	units := parseMarkdownUnits(strings.Split(doc.Content, "\n"))
	if len(units) == 0 {
		return nil
	}
	var (
		out      []*schema.Document
		buf      []mdUnit // 待合并的段落/列表/引用缓冲
		bufLen   int
		prevTail string // 上一块的尾部文本（overlap 复用）
		index    int
	)
	emit := func(text, path string) {
		if c.overlap > 0 && prevTail != "" {
			text = prevTail + "\n" + text
		}
		out = append(out, newChunkDoc(doc, index, path, text))
		prevTail = lastRunes(text, c.overlap)
		index++
	}
	flush := func() {
		if len(buf) == 0 {
			return
		}
		emit(joinUnits(buf), buf[0].path)
		buf = nil
		bufLen = 0
	}
	for _, u := range units {
		switch u.kind {
		case unitCodeBlock, unitTable:
			// 原子单元：先结算缓冲，再整体成块
			flush()
			emit(u.text, u.path)
		case unitHeading:
			// 标题为硬边界：结算当前块，标题文本不进入块正文（只存在于 headingPath）
			flush()
		default:
			buf = append(buf, u)
			bufLen += len(u.text)
			if bufLen >= c.size {
				flush()
			}
		}
	}
	flush()
	return out
}

// newChunkDoc 组装一个 chunk 文档：保留输入 MetaData，补充 chunkIndex/headingPath。
// newChunkDoc assembles a chunk document: it keeps the input MetaData and adds
// chunkIndex/headingPath.
func newChunkDoc(src *schema.Document, index int, headingPath, content string) *schema.Document {
	meta := make(map[string]any, len(src.MetaData)+2)
	for k, v := range src.MetaData {
		meta[k] = v
	}
	meta["chunkIndex"] = index
	meta["headingPath"] = headingPath
	id := src.ID
	if id != "" {
		id += "#" + strconv.Itoa(index)
	}
	return &schema.Document{ID: id, Content: content, MetaData: meta}
}

// joinUnits 将多个可合并单元以空行分隔拼接为块正文。
// joinUnits joins mergeable units into chunk text separated by blank lines.
func joinUnits(units []mdUnit) string {
	parts := make([]string, 0, len(units))
	for _, u := range units {
		parts = append(parts, u.text)
	}
	return strings.Join(parts, "\n\n")
}

// lastRunes 返回字符串末尾的 n 个字符；n 非正数或字符串为空时返回空串。
// lastRunes returns the last n characters of s; an empty string is returned when n is
// non-positive or s is empty.
func lastRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[len(runes)-n:])
}

// parseMarkdownUnits 行级解析 markdown 为原子单元序列，标题行只更新标题路径不产出单元。
// parseMarkdownUnits parses markdown line by line into atomic units; heading lines only
// update the heading path and never produce a unit.
func parseMarkdownUnits(lines []string) []mdUnit {
	var (
		units []mdUnit
		stack []string // 标题路径栈
		level []int    // 与 stack 对应的标题层级
		path  string
	)
	i, n := 0, len(lines)
	for i < n {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if lv, text, ok := parseATXHeading(line); ok {
			stack, level = pushHeading(stack, level, lv, text)
			path = strings.Join(stack, " / ")
			// 标题以硬边界标记产出：chunk 阶段据此结算当前块，标题文本不进入块正文
			units = append(units, mdUnit{kind: unitHeading, text: text, path: path})
			i++
			continue
		}
		if ch, ok := fenceOpener(line); ok {
			text, next := scanFencedBlock(lines, i, ch)
			units = append(units, mdUnit{kind: unitCodeBlock, text: text, path: path})
			i = next
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			text, next := scanTable(lines, i)
			units = append(units, mdUnit{kind: unitTable, text: text, path: path})
			i = next
			continue
		}
		if isListMarker(line) {
			text, next := scanList(lines, i)
			units = append(units, mdUnit{kind: unitList, text: text, path: path})
			i = next
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), ">") {
			text, next := scanQuote(lines, i)
			units = append(units, mdUnit{kind: unitQuote, text: text, path: path})
			i = next
			continue
		}
		text, next := scanParagraph(lines, i)
		units = append(units, mdUnit{kind: unitParagraph, text: text, path: path})
		i = next
	}
	return units
}

// parseATXHeading 解析 ATX 标题行（#..###### 后跟空格）；不满足时返回 ok=false。
// parseATXHeading parses an ATX heading line (#..###### followed by a space); ok=false
// when the line is not a heading.
func parseATXHeading(line string) (level int, text string, ok bool) {
	t := strings.TrimLeft(line, " \t")
	if t == "" || t[0] != '#' {
		return 0, "", false
	}
	i := 0
	for i < len(t) && t[i] == '#' {
		i++
	}
	if i > 6 || (i < len(t) && t[i] != ' ') {
		return 0, "", false
	}
	text = strings.TrimRight(strings.TrimSpace(t[i:]), "#")
	return i, strings.TrimSpace(text), true
}

// fenceOpener 判断行是否为围栏代码块开启行，返回围栏字符（` 或 ~）。
// fenceOpener reports whether the line opens a fenced code block, returning the fence
// character (` or ~).
func fenceOpener(line string) (byte, bool) {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return 0, false
	}
	ch := t[0]
	if ch != '`' && ch != '~' {
		return 0, false
	}
	i := 0
	for i < len(t) && t[i] == ch {
		i++
	}
	if i < 3 {
		return 0, false
	}
	return ch, true
}

// fenceCloser 判断行是否为匹配围栏字符的闭合行。
// fenceCloser reports whether the line closes a fence of the given character.
func fenceCloser(line string, ch byte) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	i := 0
	for i < len(t) && t[i] == ch {
		i++
	}
	return i >= 3
}

// scanFencedBlock 从开启行开始收集围栏代码块直至闭合行；未闭合时延伸到文档末尾。
// scanFencedBlock collects a fenced code block from its opening line to the closing
// line; an unclosed fence extends to the end of the document.
func scanFencedBlock(lines []string, start int, ch byte) (text string, next int) {
	text = lines[start]
	j := start + 1
	for j < len(lines) {
		text += "\n" + lines[j]
		if fenceCloser(lines[j], ch) {
			j++
			break
		}
		j++
	}
	return text, j
}

// scanTable 收集连续以 | 开头的表格行（含分隔行）。
// scanTable collects consecutive table rows starting with | (including the separator row).
func scanTable(lines []string, start int) (text string, next int) {
	text = lines[start]
	j := start + 1
	for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
		text += "\n" + lines[j]
		j++
	}
	return text, j
}

// isListMarker 判断行是否为列表项（- * + 或 数字. / 数字) 后跟空白）。
// isListMarker reports whether the line is a list item (- * + or number./number)
// followed by whitespace).
func isListMarker(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if len(t) < 2 {
		return false
	}
	switch t[0] {
	case '-', '*', '+':
		return t[1] == ' ' || t[1] == '\t'
	default:
		if t[0] < '0' || t[0] > '9' {
			return false
		}
		i := 0
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
		}
		if i > 9 || i >= len(t) || (t[i] != '.' && t[i] != ')') {
			return false
		}
		return i+1 < len(t) && (t[i+1] == ' ' || t[i+1] == '\t')
	}
}

// isIndentedContinuation 判断行是否为列表的缩进续行（以空白开头且非空）。
// isIndentedContinuation reports whether the line is an indented continuation of a list
// (starts with whitespace and is not blank).
func isIndentedContinuation(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t') && strings.TrimSpace(line) != ""
}

// scanList 收集列表块：列表项行 + 缩进续行；空行仅在后续仍为列表项时计入。
// scanList collects a list block: list-item lines plus indented continuations; a blank
// line is only included when the following content is still a list item.
func scanList(lines []string, start int) (text string, next int) {
	text = lines[start]
	j := start + 1
	for j < len(lines) {
		l := lines[j]
		if strings.TrimSpace(l) == "" {
			k := j + 1
			for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
				k++
			}
			if k < len(lines) && isListMarker(lines[k]) {
				j = k
				continue
			}
			break
		}
		if isListMarker(l) || isIndentedContinuation(l) {
			text += "\n" + l
			j++
			continue
		}
		break
	}
	return text, j
}

// scanQuote 收集引用块：连续 > 行；空行仅在后续仍为引用行时计入。
// scanQuote collects a quote block: consecutive > lines; a blank line is only included
// when the following content is still quoted.
func scanQuote(lines []string, start int) (text string, next int) {
	text = lines[start]
	j := start + 1
	for j < len(lines) {
		t := strings.TrimSpace(lines[j])
		if strings.HasPrefix(t, ">") {
			text += "\n" + lines[j]
			j++
			continue
		}
		if t == "" {
			k := j + 1
			for k < len(lines) && strings.TrimSpace(lines[k]) == "" {
				k++
			}
			if k < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[k]), ">") {
				j = k
				continue
			}
		}
		break
	}
	return text, j
}

// scanParagraph 收集普通段落：连续非空且非结构起始行。
// scanParagraph collects a plain paragraph: consecutive non-blank lines that do not
// start a structural block.
func scanParagraph(lines []string, start int) (text string, next int) {
	text = lines[start]
	j := start + 1
	for j < len(lines) && !isStructureStart(lines[j]) {
		text += "\n" + lines[j]
		j++
	}
	return text, j
}

// isStructureStart 判断行是否为结构起始行（空行/标题/围栏/表格/列表/引用）。
// isStructureStart reports whether the line starts a structural block (blank line,
// heading, fence, table, list or quote).
func isStructureStart(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return true
	}
	if _, _, ok := parseATXHeading(line); ok {
		return true
	}
	if _, ok := fenceOpener(line); ok {
		return true
	}
	if strings.HasPrefix(t, "|") {
		return true
	}
	if isListMarker(line) {
		return true
	}
	return strings.HasPrefix(t, ">")
}

// pushHeading 更新标题路径栈：弹出层级不低于当前标题的栈顶，再压入当前标题。
// pushHeading updates the heading path stack: entries whose level is not lower than the
// current heading are popped, then the current heading is pushed.
func pushHeading(stack []string, levels []int, level int, text string) ([]string, []int) {
	for len(levels) > 0 && levels[len(levels)-1] >= level {
		levels = levels[:len(levels)-1]
		stack = stack[:len(stack)-1]
	}
	return append(stack, text), append(levels, level)
}

package llm

import (
	// 标准库
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// 默认切片参数（size=800, overlap=100）：行为参数注册表中 chunking.size /
// chunking.overlap 的默认值，构造参数非正数时回退使用。
const (
	defaultChunkSize    = 800
	defaultChunkOverlap = 100
)

// mdUnitKind 标识 markdown 解析出的原子单元类型。
type mdUnitKind int

const (
	// unitHeading 标题行（硬边界标记，自身不产出块正文）
	unitHeading mdUnitKind = iota
	// unitParagraph 普通段落
	unitParagraph
	// unitCodeBlock 围栏代码块
	unitCodeBlock
	// unitTable 表格块
	unitTable
	// unitList 列表块
	unitList
	// unitQuote 引用块
	unitQuote
)

// mdUnit 是行级解析 markdown 得到的不可拆散原子单元。
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
//   - 仅当一块因达到 chunking.size 在段落中途被截断时，其下一块才按 chunking.overlap
//     字符复用该块尾部文本衔接（重叠至多取该块长度的一半）；命中标题、原子单元成块等
//     自然边界处不存在被切开的接缝，下一块不复用上一块文本。
type MarkdownChunker struct {
	size    int // 亲和目标长度（字符）
	overlap int // 相邻块重叠长度（字符）
}

// NewMarkdownChunker 构造 MarkdownChunker。size <= 0 时回退默认 800，overlap < 0 时按 0 处理。
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
func (c *MarkdownChunker) Transform(_ context.Context, src []*schema.Document, _ ...document.TransformerOption) ([]*schema.Document, error) {
	var out []*schema.Document
	for _, doc := range src {
		out = append(out, c.chunk(doc)...)
	}
	return out, nil
}

// chunk 将单个文档切为多个 chunk；空文档/纯空白返回空切片。
func (c *MarkdownChunker) chunk(doc *schema.Document) []*schema.Document {
	units := parseMarkdownUnits(strings.Split(doc.Content, "\n"))
	if len(units) == 0 {
		return nil
	}
	var (
		out      []*schema.Document
		buf      []mdUnit // 待合并的段落/列表/引用缓冲
		bufLen   int
		prevTail string // 上一块可复用的尾部文本（overlap，已按上限封顶）
		prevCut  bool   // 上一块是否因达到 size 在段落中途被截断（只有此时才存在待弥合的接缝）
		index    int
	)
	// emit 结算一块。carry 表示本块紧接在上一块被截断的位置之后，需要复用其尾部文本：
	// overlap 的用途是弥合被 size 切开的接缝——接缝两侧本是同一段连续文本，重叠让恰好落在
	// 接缝上的查询能命中任意一侧；而命中标题、原子单元成块、文档末尾都是自然边界，
	// 跨过去只会把上一节的文本贴进本节正文，故只有被 size 截断时才衔接。
	emit := func(text, path string, carry bool) {
		if carry && prevTail != "" {
			text = prevTail + "\n" + text
		}
		out = append(out, newChunkDoc(doc, index, path, text))
		// 尾部文本至多取本块长度的一半：overlap 是弥合接缝的手段，只应复读一小部分；而
		// size 与 overlap 由行为参数各自配置，若 overlap 被调得过大（不小于本块长度），
		// 整块复读会让下一块失去全部独有正文信息
		prevTail = lastRunes(text, min(c.overlap, utf8.RuneCountInString(text)/2))
		index++
	}
	// flush 结算缓冲并成块；cut 表示本次结算是否因达到 size 把内容截断在段落中途，
	// 该标记传给下一块，决定它能否复用本块尾部
	flush := func(cut bool) {
		if len(buf) == 0 {
			return
		}
		emit(joinUnits(buf), buf[0].path, prevCut)
		prevCut = cut
		buf = nil
		bufLen = 0
	}
	for _, u := range units {
		switch u.kind {
		case unitCodeBlock, unitTable:
			// 原子单元：先结算缓冲，再整体成块；单元自身完整，前后都不重叠
			flush(false)
			emit(u.text, u.path, false)
			prevCut = false
		case unitHeading:
			// 标题为硬边界：结算当前块，标题文本不进入块正文（只存在于 headingPath）；
			// 边界之后不再复用上一块尾部，硬边界不承载跨节重叠
			flush(false)
			prevCut = false
		default:
			buf = append(buf, u)
			bufLen += len(u.text)
			if bufLen >= c.size {
				// 段落中途截断：本块尾部可与下一块衔接
				flush(true)
			}
		}
	}
	flush(false)
	return out
}

// newChunkDoc 组装一个 chunk 文档：保留输入 MetaData，补充 chunkIndex/headingPath。
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
func joinUnits(units []mdUnit) string {
	parts := make([]string, 0, len(units))
	for _, u := range units {
		parts = append(parts, u.text)
	}
	return strings.Join(parts, "\n\n")
}

// lastRunes 返回字符串末尾的 n 个字符；n 非正数或字符串为空时返回空串。
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
func isIndentedContinuation(line string) bool {
	return line != "" && (line[0] == ' ' || line[0] == '\t') && strings.TrimSpace(line) != ""
}

// scanList 收集列表块：列表项行 + 缩进续行；空行仅在后续仍为列表项时计入。
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
func pushHeading(stack []string, levels []int, level int, text string) ([]string, []int) {
	for len(levels) > 0 && levels[len(levels)-1] >= level {
		levels = levels[:len(levels)-1]
		stack = stack[:len(stack)-1]
	}
	return append(stack, text), append(levels, level)
}

// ChunkUnitKind 是分块正文中识别出的结构单元类型（chunkUnitKind 枚举类的取值类型）。
type ChunkUnitKind string

// 分块结构单元类型常量。
const (
	// ChunkUnitParagraph 普通段落
	ChunkUnitParagraph ChunkUnitKind = "paragraph"
	// ChunkUnitCode 围栏代码块（原子单元，超长也不切分）
	ChunkUnitCode ChunkUnitKind = "code"
	// ChunkUnitTable 表格块（原子单元，超长也不切分）
	ChunkUnitTable ChunkUnitKind = "table"
	// ChunkUnitList 列表块
	ChunkUnitList ChunkUnitKind = "list"
	// ChunkUnitQuote 引用块
	ChunkUnitQuote ChunkUnitKind = "quote"
)

// chunkUnitKindValues 是 chunkUnitKind 枚举类的取值空间，声明顺序即枚举类条目顺序与展示顺序。
var chunkUnitKindValues = []ChunkUnitKind{
	ChunkUnitParagraph, ChunkUnitCode, ChunkUnitTable, ChunkUnitList, ChunkUnitQuote,
}

// mdUnitKindToChunkUnitKind 把行级解析得到的内部单元类型映射为对外的结构单元类型；标题单元
// 返回 ok=false（标题为硬边界，只存在于 headingPath，绝不进入分块正文）。
func mdUnitKindToChunkUnitKind(kind mdUnitKind) (ChunkUnitKind, bool) {
	switch kind {
	case unitParagraph:
		return ChunkUnitParagraph, true
	case unitCodeBlock:
		return ChunkUnitCode, true
	case unitTable:
		return ChunkUnitTable, true
	case unitList:
		return ChunkUnitList, true
	case unitQuote:
		return ChunkUnitQuote, true
	default:
		return "", false
	}
}

// DescribeChunk 复用本包的行级 markdown 解析识别分块正文包含的结构单元类型：去重后按
// chunkUnitKindValues 的声明顺序稳定返回；无单元时返回空切片（而非 nil）。识别只针对正文，
// 标题单元不参与（见 mdUnitKindToChunkUnitKind）。
func DescribeChunk(content string) []ChunkUnitKind {
	units := parseMarkdownUnits(strings.Split(content, "\n"))
	found := make(map[ChunkUnitKind]struct{}, len(chunkUnitKindValues))
	for _, u := range units {
		if kind, ok := mdUnitKindToChunkUnitKind(u.kind); ok {
			found[kind] = struct{}{}
		}
	}
	out := make([]ChunkUnitKind, 0, len(found))
	for _, kind := range chunkUnitKindValues {
		if _, ok := found[kind]; ok {
			out = append(out, kind)
		}
	}
	return out
}

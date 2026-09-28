package llm

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/schema"
)

// TestMarkdownChunker_HeadingsAreBoundaries 测试标题为硬边界：正文不含标题行，
// 每个标题下的内容独立成块，headingPath 与标题层级一致。
func TestMarkdownChunker_HeadingsAreBoundaries(t *testing.T) {
	doc := &schema.Document{
		Content: "# Root\npara a\n## Sub\npara b\n### Leaf\npara c",
	}
	c := NewMarkdownChunker(800, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(got))
	}
	wantPaths := []string{"Root", "Root / Sub", "Root / Sub / Leaf"}
	wantContents := []string{"para a", "para b", "para c"}
	for i, d := range got {
		if p, _ := d.MetaData["headingPath"].(string); p != wantPaths[i] {
			t.Errorf("chunk[%d] headingPath = %q, want %q", i, p, wantPaths[i])
		}
		if d.Content != wantContents[i] {
			t.Errorf("chunk[%d] content = %q, want %q", i, d.Content, wantContents[i])
		}
		if strings.Contains(d.Content, "#") {
			t.Errorf("chunk[%d] content repeats heading text: %q", i, d.Content)
		}
	}
}

// TestMarkdownChunker_CodeBlockAtomic 测试围栏代码块为原子单元，整体超长也不硬切。
func TestMarkdownChunker_CodeBlockAtomic(t *testing.T) {
	// size=10 远小于代码块长度，但代码块必须整体成一块
	content := "## Code\n```go\nline one\nline two\nline three\n```\nAfter"
	doc := &schema.Document{Content: content}
	c := NewMarkdownChunker(10, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	if got[0].Content != "```go\nline one\nline two\nline three\n```" {
		t.Errorf("chunk[0] content = %q, want the whole code block", got[0].Content)
	}
	if p, _ := got[0].MetaData["headingPath"].(string); p != "Code" {
		t.Errorf("chunk[0] headingPath = %q, want %q", p, "Code")
	}
	if got[1].Content != "After" {
		t.Errorf("chunk[1] content = %q, want %q", got[1].Content, "After")
	}
}

// TestMarkdownChunker_UnclosedFenceExtends 测试未闭合的围栏延伸到文档末尾，
// 其内部的标题行不会成为边界。
func TestMarkdownChunker_UnclosedFenceExtends(t *testing.T) {
	content := "# Doc\n```\nline1\n# NotAHeading\nline2\n"
	doc := &schema.Document{Content: content}
	c := NewMarkdownChunker(10, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("chunk count = %d, want 1 (unclosed fence must not split)", len(got))
	}
	if p, _ := got[0].MetaData["headingPath"].(string); p != "Doc" {
		t.Errorf("headingPath = %q, want %q", p, "Doc")
	}
	if !strings.Contains(got[0].Content, "# NotAHeading") {
		t.Errorf("chunk content lost the in-fence heading line: %q", got[0].Content)
	}
}

// TestMarkdownChunker_TableAtomic 测试表格块为原子单元。
func TestMarkdownChunker_TableAtomic(t *testing.T) {
	content := "## Table\n| a | b |\n|---|---|\n| 1 | 2 |\nTail"
	doc := &schema.Document{Content: content}
	c := NewMarkdownChunker(10, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	if got[0].Content != "| a | b |\n|---|---|\n| 1 | 2 |" {
		t.Errorf("chunk[0] content = %q, want the whole table", got[0].Content)
	}
	if got[1].Content != "Tail" {
		t.Errorf("chunk[1] content = %q, want %q", got[1].Content, "Tail")
	}
}

// TestMarkdownChunker_MergesToTargetSize 测试普通段落按 size 亲和目标合并成块。
func TestMarkdownChunker_MergesToTargetSize(t *testing.T) {
	content := strings.Join([]string{"aaaa", "bbbb", "cccc", "dddd", "eeee", "ffff"}, "\n\n")
	doc := &schema.Document{Content: content}
	c := NewMarkdownChunker(20, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	// 每段 4 字符，5 段累计 20 达 size 触发结算：块 0 = aaaa..eeee，块 1 = ffff
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	if got[0].Content != "aaaa\n\nbbbb\n\ncccc\n\ndddd\n\neeee" {
		t.Errorf("chunk[0] content = %q, want the first five paragraphs", got[0].Content)
	}
	if got[1].Content != "ffff" {
		t.Errorf("chunk[1] content = %q, want %q", got[1].Content, "ffff")
	}
}

// TestMarkdownChunker_Overlap 测试相邻块按 overlap 字符重叠衔接（复用上一块尾部）。
func TestMarkdownChunker_Overlap(t *testing.T) {
	content := strings.Join([]string{"aaaa", "bbbb", "cccc", "dddd", "eeee", "ffff"}, "\n\n")
	doc := &schema.Document{Content: content}
	c := NewMarkdownChunker(20, 2)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	// 块 0 尾部 2 字符为 "ee"，块 1 以 "ee\n" 开头
	if !strings.HasPrefix(got[1].Content, "ee\n") {
		t.Errorf("chunk[1] content = %q, want overlap prefix %q", got[1].Content, "ee\n")
	}
}

// TestMarkdownChunker_NoOverlapAcrossHeading 测试标题硬边界处不施加重叠：上一节正文
// 短于 overlap 时，不得整块复读到下一节开头。回归用例——曾把「说明」整段贴进「内容」块。
func TestMarkdownChunker_NoOverlapAcrossHeading(t *testing.T) {
	content := strings.Join([]string{
		"# 标题",
		"## 说明",
		"这是一个无聊的故事，通过无限递归地方式，讲一个故事重复下去，用来逗弄小孩子的。",
		"## 内容",
		"从前有座山，山里有座庙。",
	}, "\n\n")
	doc := &schema.Document{Content: content}
	// overlap(100) 远大于「说明」正文长度(39)，旧实现会把整段复读到「内容」块开头
	c := NewMarkdownChunker(800, 100)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	if got[1].Content != "从前有座山，山里有座庙。" {
		t.Errorf("chunk[1] content = %q, want the 内容 body without any overlap prefix", got[1].Content)
	}
}

// TestMarkdownChunker_OverlapCappedAtHalfPrevious 测试重叠长度按上一块的一半封顶：
// overlap 被配置得不小于上一块长度时，只复读上一块尾部的一半而非整块。
func TestMarkdownChunker_OverlapCappedAtHalfPrevious(t *testing.T) {
	content := strings.Join([]string{"aaaa", "bbbb", "cccc", "dddd"}, "\n\n")
	doc := &schema.Document{Content: content}
	// size(8) 触发段落中途截断，overlap(100) 远大于上一块长度(9)
	c := NewMarkdownChunker(8, 100)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(got))
	}
	if got[0].Content != "aaaa\n\nbbbb" {
		t.Fatalf("chunk[0] content = %q, want the first two paragraphs", got[0].Content)
	}
	// 上一块长 10 字符，封顶后重叠至多 5 字符；重叠是字符级尾部截取，切点可落在
	// 空白处，故此处取到 "\nbbbb"。关键是不能整块复读
	if got[1].Content != "\nbbbb\ncccc\n\ndddd" {
		t.Errorf("chunk[1] content = %q, want %q", got[1].Content, "\nbbbb\ncccc\n\ndddd")
	}
	if strings.HasPrefix(got[1].Content, got[0].Content) {
		t.Errorf("chunk[1] content = %q, must not repeat the whole previous chunk", got[1].Content)
	}
}

// TestMarkdownChunker_MetaDataPreserved 测试输入 MetaData 被保留并补充 chunkIndex。
func TestMarkdownChunker_MetaDataPreserved(t *testing.T) {
	doc := &schema.Document{
		ID:       "doc-1",
		Content:  "# H\nbody",
		MetaData: map[string]any{"source": "test"},
	}
	c := NewMarkdownChunker(800, 0)
	got, err := c.Transform(context.Background(), []*schema.Document{doc})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("chunk count = %d, want 1", len(got))
	}
	if got[0].MetaData["source"] != "test" {
		t.Errorf("MetaData lost input key %q", "source")
	}
	if idx, ok := got[0].MetaData["chunkIndex"].(int); !ok || idx != 0 {
		t.Errorf("chunkIndex = %v, want 0", got[0].MetaData["chunkIndex"])
	}
	if got[0].ID != "doc-1#0" {
		t.Errorf("chunk ID = %q, want %q", got[0].ID, "doc-1#0")
	}
}

// TestMarkdownChunker_EmptyDocument 测试空文档/纯空白文档不产出任何块。
func TestMarkdownChunker_EmptyDocument(t *testing.T) {
	c := NewMarkdownChunker(800, 100)
	for _, content := range []string{"", "   \n  \n"} {
		got, err := c.Transform(context.Background(), []*schema.Document{{Content: content}})
		if err != nil {
			t.Fatalf("Transform() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("Transform(%q) produced %d chunks, want 0", content, len(got))
		}
	}
}

// TestNewMarkdownChunker_Defaults 测试非法参数回退默认值（size<=0 → 800，overlap<0 → 0）。
func TestNewMarkdownChunker_Defaults(t *testing.T) {
	c := NewMarkdownChunker(0, -3)
	if c.size != 800 {
		t.Errorf("size = %d, want 800 (default)", c.size)
	}
	if c.overlap != 0 {
		t.Errorf("overlap = %d, want 0 (clamped)", c.overlap)
	}
}

// TestDescribeChunk 测试分块正文的结构单元识别：去重、按声明顺序返回，标题不参与，
// 空正文返回空切片。
func TestDescribeChunk(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []ChunkUnitKind
	}{
		{
			name:    "空正文",
			content: "",
			want:    []ChunkUnitKind{},
		},
		{
			name:    "纯段落",
			content: "para one\n\npara two",
			want:    []ChunkUnitKind{ChunkUnitParagraph},
		},
		{
			name:    "标题不参与识别",
			content: "## Heading\n\npara",
			want:    []ChunkUnitKind{ChunkUnitParagraph},
		},
		{
			name:    "段落与代码块并按声明顺序排列",
			content: "```go\ncode\n```\n\npara",
			want:    []ChunkUnitKind{ChunkUnitParagraph, ChunkUnitCode},
		},
		{
			name:    "重复单元去重且表格为原子单元",
			content: "| a | b |\n|---|---|\n\npara one\n\npara two",
			want:    []ChunkUnitKind{ChunkUnitParagraph, ChunkUnitTable},
		},
		{
			name:    "列表与引用",
			content: "- item one\n- item two\n\n> quoted",
			want:    []ChunkUnitKind{ChunkUnitList, ChunkUnitQuote},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DescribeChunk(tc.content)
			if len(got) != len(tc.want) {
				t.Fatalf("DescribeChunk() = %v, want %v", got, tc.want)
			}
			for i, kind := range got {
				if kind != tc.want[i] {
					t.Fatalf("DescribeChunk()[%d] = %q, want %q", i, kind, tc.want[i])
				}
			}
		})
	}
}

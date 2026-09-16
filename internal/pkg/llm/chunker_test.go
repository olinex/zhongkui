package llm

import (
	// stdlib
	"context"
	"strings"
	"testing"

	// third-party
	"github.com/cloudwego/eino/schema"
)

// TestMarkdownChunker_HeadingsAreBoundaries 测试标题为硬边界：正文不含标题行，
// 每个标题下的内容独立成块，headingPath 与标题层级一致。
// TestMarkdownChunker_HeadingsAreBoundaries verifies headings are hard boundaries: the
// chunk body never repeats heading lines, content under each heading becomes its own
// chunk, and headingPath matches the heading hierarchy.
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
// TestMarkdownChunker_CodeBlockAtomic verifies a fenced code block is atomic and stays
// whole even when oversized.
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
// TestMarkdownChunker_UnclosedFenceExtends verifies an unclosed fence extends to the end
// of the document and heading-looking lines inside it never become boundaries.
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
// TestMarkdownChunker_TableAtomic verifies a table block is an atomic unit.
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
// TestMarkdownChunker_MergesToTargetSize verifies plain paragraphs merge toward the size
// affinity target.
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
// TestMarkdownChunker_Overlap verifies adjacent chunks overlap by overlap characters
// (reusing the previous chunk tail).
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

// TestMarkdownChunker_MetaDataPreserved 测试输入 MetaData 被保留并补充 chunkIndex。
// TestMarkdownChunker_MetaDataPreserved verifies the input MetaData is preserved and
// chunkIndex is added.
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
// TestMarkdownChunker_EmptyDocument verifies an empty or whitespace-only document yields
// no chunks.
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
// TestNewMarkdownChunker_Defaults verifies invalid parameters fall back to defaults
// (size<=0 → 800, overlap<0 → 0).
func TestNewMarkdownChunker_Defaults(t *testing.T) {
	c := NewMarkdownChunker(0, -3)
	if c.size != 800 {
		t.Errorf("size = %d, want 800 (default)", c.size)
	}
	if c.overlap != 0 {
		t.Errorf("overlap = %d, want 0 (clamped)", c.overlap)
	}
}

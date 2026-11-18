package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseSchemaPackage 校验结构体解析、表过滤、嵌入展开与可空性判定。
func TestParseSchemaPackage(t *testing.T) {
	dir := t.TempDir()
	source := `package sample

// Widget 是示例表。
type Widget struct {
	Base
	Name string ` + "`gorm:\"type:VARCHAR(32);not null\"`" + ` // 名称
	Note string ` + "`gorm:\"type:TEXT\"`" + ` // 备注
}

// TableName 返回示例表名。
func (w *Widget) TableName() string { return "widgets" }

// Base 是嵌入基类。
type Base struct {
	ID uint ` + "`gorm:\"primarykey\"`" + `
}

// ValueObject 不是表。
type ValueObject struct {
	Foo string
}
`
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("写入测试源码失败: %v", err)
	}

	parsed, err := parseSchemaPackage(dir, map[string]string{"widgets": "postgres"})
	if err != nil {
		t.Fatalf("parseSchemaPackage 返回错误: %v", err)
	}
	byName := make(map[string]schemaType, len(parsed))
	for _, item := range parsed {
		byName[item.Name] = item
	}
	widget, ok := byName["Widget"]
	if !ok {
		t.Fatalf("未解析到 Widget: %+v", parsed)
	}
	if _, ok := byName["Base"]; ok {
		t.Fatalf("未声明 TableName 的 Base 不应作为数据表导出: %+v", parsed)
	}
	if len(widget.Fields) != 3 {
		t.Fatalf("字段数 = %d, want 3: %+v", len(widget.Fields), widget.Fields)
	}
	if widget.Fields[0].Column != "id" || !widget.Fields[0].Primary {
		t.Errorf("嵌入主键展开错误: %+v", widget.Fields[0])
	}
	if widget.Fields[1].Column != "name" || widget.Fields[1].Nullable != "否" {
		t.Errorf("名称字段解析错误: %+v", widget.Fields[1])
	}
	if widget.Fields[2].Column != "note" || widget.Fields[2].Nullable != "是" {
		t.Errorf("备注字段可空性错误: %+v", widget.Fields[2])
	}
}

// TestSchemaNullableClickHouse 校验 ClickHouse 表按 Nullable 类型判定可空性。
func TestSchemaNullableClickHouse(t *testing.T) {
	dir := t.TempDir()
	source := `package sample

type Packet struct {
	Fixed string ` + "`gorm:\"type:LowCardinality(String)\"`" + `
	Maybe string ` + "`gorm:\"type:Nullable(String)\"`" + `
}

// TableName 返回示例表名。
func (p *Packet) TableName() string { return "packets" }
`
	if err := os.WriteFile(filepath.Join(dir, "packet.go"), []byte(source), 0o644); err != nil {
		t.Fatalf("写入测试源码失败: %v", err)
	}
	parsed, err := parseSchemaPackage(dir, map[string]string{"packets": "clickhouse"})
	if err != nil {
		t.Fatalf("parseSchemaPackage 返回错误: %v", err)
	}
	fields := parsed[0].Fields
	if fields[0].Nullable != "否" {
		t.Errorf("非 Nullable 列应不可空: %+v", fields[0])
	}
	if fields[1].Nullable != "是" {
		t.Errorf("Nullable 列应可空: %+v", fields[1])
	}
}

// TestRenderSchemaPage 校验表页包含字段表、索引与包参考链接。
func TestRenderSchemaPage(t *testing.T) {
	model := schemaModel{
		Domain: "core",
		Name:   "Widget",
		Table:  "widgets",
		Doc:    "示例表。",
		Fields: []schemaField{
			{Column: "id", GoType: "uint", Nullable: "否", Default: "-", Comment: "主键", Primary: true},
			{Column: "name|alias", GoType: "VARCHAR(32)", Nullable: "否", Default: "-", Comment: "名称"},
		},
	}
	content := renderSchemaPage(model)
	for _, want := range []string{"## 表用途", "示例表。", "name\\|alias", "主键：`id`", "../internal/models/core.md"} {
		if !strings.Contains(content, want) {
			t.Errorf("表页缺少 %q:\n%s", want, content)
		}
	}
}

// TestSchemaKnowledgeDocuments 校验数据表知识文档集与已迁移表同源、确定性，且不含仓库相对链接。
func TestSchemaKnowledgeDocuments(t *testing.T) {
	docs, err := SchemaKnowledgeDocuments(repoRoot(t))
	if err != nil {
		t.Fatalf("SchemaKnowledgeDocuments error = %v", err)
	}
	if len(docs) < 10 {
		t.Fatalf("文档数 = %d，期望至少 10 张已迁移表", len(docs))
	}
	seen := make(map[string]bool, len(docs))
	for _, doc := range docs {
		if doc.Key == "" || doc.Title == "" || doc.Content == "" {
			t.Errorf("文档缺少键、标题或正文: %+v", doc)
		}
		if seen[doc.Title] {
			t.Errorf("文档标题重复: %q", doc.Title)
		}
		seen[doc.Title] = true
		if !strings.Contains(doc.Content, "## 字段") {
			t.Errorf("文档 %q 缺少字段小节", doc.Title)
		}
		if strings.Contains(doc.Content, "../../internal/models/") {
			t.Errorf("文档 %q 不应包含仓库相对链接", doc.Title)
		}
	}
	again, err := SchemaKnowledgeDocuments(repoRoot(t))
	if err != nil {
		t.Fatalf("SchemaKnowledgeDocuments(second) error = %v", err)
	}
	if len(again) != len(docs) {
		t.Fatalf("两次生成文档数不一致: %d vs %d", len(docs), len(again))
	}
	for i := range docs {
		if docs[i] != again[i] {
			t.Errorf("第 %d 篇文档两次生成不一致", i)
		}
	}
}

// TestStructTagGet 校验字段标签读取。
func TestStructTagGet(t *testing.T) {
	tag := structTagInfo{raw: `gorm:"primarykey"`}
	if got := tag.Get("gorm"); got != "primarykey" {
		t.Fatalf("Get(gorm) = %q", got)
	}
	if !hasGormKey(parseGormTag(tag.Get("gorm")), "primaryKey") {
		t.Fatalf("primaryKey 未命中")
	}
}

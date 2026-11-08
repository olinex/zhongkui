package cmd

import (
	// 标准库
	"testing"
)

// TestNarrativeKnowledgeDocs_FallbackToSnapshot 校验无源码树（发布二进制）时叙述页回退到
// 编译期快照：目录内不存在 go.mod 即视为运行镜像环境。
func TestNarrativeKnowledgeDocs_FallbackToSnapshot(t *testing.T) {
	docs, err := narrativeKnowledgeDocs(t.TempDir())
	if err != nil {
		t.Fatalf("narrativeKnowledgeDocs error = %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("叙述页快照为空")
	}
	for _, doc := range docs {
		if doc.Key == "" || doc.Title == "" || doc.Content == "" {
			t.Fatalf("叙述页存在空字段：%+v", doc)
		}
	}
}

// TestSchemaKnowledgeDocs_FallbackToSnapshot 校验无源码树时数据表知识回退到编译期快照。
func TestSchemaKnowledgeDocs_FallbackToSnapshot(t *testing.T) {
	docs, err := schemaKnowledgeDocs(t.TempDir())
	if err != nil {
		t.Fatalf("schemaKnowledgeDocs error = %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("数据表快照为空")
	}
	for _, doc := range docs {
		if doc.Key == "" || doc.Title == "" || doc.Content == "" {
			t.Fatalf("数据表存在空字段：%+v", doc)
		}
	}
}

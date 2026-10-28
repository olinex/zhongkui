// 本文件是「分区隔离」守卫测试：验证生成知识文档在 knowledge_documents 中按 format 分区
// 严格隔离——native_api 与 generated 分区的写读互不串扰，loadPartitionDocuments 只返回指定
// format 的行（写入侧以 format 落库，读取侧以 format 过滤）。
package knowledge

import (
	// 标准库
	"context"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
)

// assertPartitionTitles 断言指定 format 分区的读取结果恰好包含 want 中的标题，
// 且不包含 forbid 中的标题（即未串入其它分区）。
func assertPartitionTitles(t *testing.T, partition string, got []existingGeneratedDoc, want []string, forbid ...string) {
	t.Helper()
	gotTitles := make(map[string]bool, len(got))
	for _, doc := range got {
		gotTitles[doc.Title] = true
	}
	if len(got) != len(want) {
		t.Errorf("%s 分区返回 %d 行，期望 %d 行，实际标题集合 = %v", partition, len(got), len(want), gotTitles)
	}
	for _, title := range want {
		if !gotTitles[title] {
			t.Errorf("%s 分区缺少行 %q", partition, title)
		}
	}
	for _, title := range forbid {
		if gotTitles[title] {
			t.Errorf("%s 分区不应包含其它分区行 %q（format 未严格隔离）", partition, title)
		}
	}
}

// TestGeneratedDocsPartitionIsolation 校验同一命名空间下 native_api 与 generated 两分区在
// 读侧按 format 严格隔离：两类文档同库共存时，各自查询只返回本分区行。
func TestGeneratedDocsPartitionIsolation(t *testing.T) {
	gdb := aitest.NewKnowledgeDB(t)

	if docconv.FormatNativeApi == docconv.FormatGenerated {
		t.Fatalf("native_api 与 generated 分区取值必须不同，实际均为 %q", string(docconv.FormatGenerated))
	}

	const namespace = "partition-isolation-ns"
	rows := []struct {
		title  string
		format docconv.Format
		hash   string
	}{
		{"native-doc-1", docconv.FormatNativeApi, "hash-native-1"},
		{"native-doc-2", docconv.FormatNativeApi, "hash-native-2"},
		{"generated-doc-1", docconv.FormatGenerated, "hash-generated-1"},
		{"generated-doc-2", docconv.FormatGenerated, "hash-generated-2"},
	}
	for _, row := range rows {
		if err := gdb.Exec(
			"INSERT INTO knowledge_documents (namespace_name, title, format, hash, deleted_at) VALUES (?, ?, ?, ?, NULL)",
			namespace, row.title, string(row.format), row.hash,
		).Error; err != nil {
			t.Fatalf("插入分区行 %q(format=%s) 失败：%v", row.title, row.format, err)
		}
	}

	knowledge := &Knowledge{db: gdb}
	ctx := context.Background()

	native, err := knowledge.loadPartitionDocuments(ctx, namespace, docconv.FormatNativeApi)
	if err != nil {
		t.Fatalf("loadPartitionDocuments(native_api) 错误：%v", err)
	}
	assertPartitionTitles(t, "native_api", native,
		[]string{"native-doc-1", "native-doc-2"},
		"generated-doc-1", "generated-doc-2")

	generated, err := knowledge.loadPartitionDocuments(ctx, namespace, docconv.FormatGenerated)
	if err != nil {
		t.Fatalf("loadPartitionDocuments(generated) 错误：%v", err)
	}
	assertPartitionTitles(t, "generated", generated,
		[]string{"generated-doc-1", "generated-doc-2"},
		"native-doc-1", "native-doc-2")
}

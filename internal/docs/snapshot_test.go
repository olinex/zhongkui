package docs

import (
	// 标准库
	"bytes"
	"testing"
)

// TestEmbeddedRAGSnapshot_MatchSource 守卫编译期快照与源码一致：发布二进制只能用快照，
// 快照一旦落后于代码声明，zki sync docs / sync schema 注入的知识就会失真。
// 更新快照请运行 make docs。
func TestEmbeddedRAGSnapshot_MatchSource(t *testing.T) {
	fresh, err := BuildRAGSnapshot(repoRoot(t))
	if err != nil {
		t.Fatalf("BuildRAGSnapshot error = %v", err)
	}
	data, err := MarshalRAGSnapshot(fresh)
	if err != nil {
		t.Fatalf("MarshalRAGSnapshot error = %v", err)
	}
	if !bytes.Equal(data, embeddedRAGSnapshot) {
		t.Fatalf("RAG 快照已过期，请运行 make docs 重新生成")
	}
	if len(fresh.Narratives) == 0 || len(fresh.Schemas) == 0 {
		t.Fatalf("快照内容为空：叙述页 %d 篇、数据表 %d 张", len(fresh.Narratives), len(fresh.Schemas))
	}
}

// TestEmbeddedRAGSnapshot_Loadable 校验嵌入式快照可解码且版本兼容。
func TestEmbeddedRAGSnapshot_Loadable(t *testing.T) {
	snapshot, err := EmbeddedRAGSnapshot()
	if err != nil {
		t.Fatalf("EmbeddedRAGSnapshot error = %v", err)
	}
	if snapshot.Version != ragSnapshotVersion {
		t.Fatalf("快照版本 = %d，期望 %d", snapshot.Version, ragSnapshotVersion)
	}
	for _, doc := range snapshot.Narratives {
		if doc.Key == "" || doc.Title == "" || doc.Content == "" {
			t.Fatalf("叙述页存在空字段：%+v", doc)
		}
	}
	for _, doc := range snapshot.Schemas {
		if doc.Key == "" || doc.Title == "" || doc.Content == "" {
			t.Fatalf("数据表存在空字段：%+v", doc)
		}
	}
}

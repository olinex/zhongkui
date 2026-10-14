// 本文件覆盖知识检索 small-to-big 展开的纯逻辑：mergeExpandedResults 的同文档收敛、
// 整篇替换与 Complete 标记、单篇超限与累计预算超限的降级、关闭开关与未覆盖文档的原样保留，
// 以及 resultDocumentIDs 的去重与非法 ID 过滤。全部用例不依赖 PostgreSQL。

package ai

import (
	// 标准库
	"testing"
)

// TestMergeExpandedResults 覆盖整篇展开合并的四种情形。
func TestMergeExpandedResults(t *testing.T) {
	t.Run("同文档只留首条并替换为整篇", func(t *testing.T) {
		results := []*RetrievalResult{
			{DocumentID: "1", ChunkIndex: 0, Content: "chunk-0"},
			{DocumentID: "1", ChunkIndex: 1, Content: "chunk-1"},
			{DocumentID: "2", ChunkIndex: 0, Content: "other"},
		}
		fullTexts := map[uint]string{1: "整篇文档正文"}

		got := mergeExpandedResults(results, fullTexts, 4000)
		if len(got) != 2 {
			t.Fatalf("merged length = %d, want 2 (same-document hits collapse)", len(got))
		}
		if got[0].DocumentID != "1" || got[0].Content != "整篇文档正文" || !got[0].Complete {
			t.Errorf("first hit = %+v, want doc 1 expanded with Complete=true", got[0])
		}
		if got[0].ChunkIndex != 0 {
			t.Errorf("first hit chunkIndex = %d, want the surviving first hit (0)", got[0].ChunkIndex)
		}
		if got[1].DocumentID != "2" || got[1].Complete || got[1].Content != "other" {
			t.Errorf("second hit = %+v, want doc 2 unchanged", got[1])
		}
		// 输入切片不得被就地修改（展开走克隆）
		if results[0].Content != "chunk-0" || results[0].Complete {
			t.Errorf("original result mutated = %+v, want it untouched", results[0])
		}
	})

	t.Run("整篇超过长度上限时退回分块", func(t *testing.T) {
		results := []*RetrievalResult{{DocumentID: "1", ChunkIndex: 0, Content: "chunk-0"}}
		fullTexts := map[uint]string{1: "一二三四五"}

		got := mergeExpandedResults(results, fullTexts, 3)
		if len(got) != 1 || got[0].Content != "chunk-0" || got[0].Complete {
			t.Errorf("merged = %+v, want the original chunk kept when over the limit", got)
		}
	})

	t.Run("累计预算用尽后其后文档退回分块", func(t *testing.T) {
		results := []*RetrievalResult{
			{DocumentID: "1", ChunkIndex: 0, Content: "chunk-a"},
			{DocumentID: "2", ChunkIndex: 0, Content: "chunk-b"},
		}
		fullTexts := map[uint]string{1: "一二三", 2: "四五六"}

		// 预算 5：首篇（3）可展开，累计后余 2 不足以展开次篇（3），次篇降级为原分块
		got := mergeExpandedResults(results, fullTexts, 5)
		if len(got) != 2 {
			t.Fatalf("merged length = %d, want 2", len(got))
		}
		if got[0].Content != "一二三" || !got[0].Complete {
			t.Errorf("first hit = %+v, want doc 1 expanded within budget", got[0])
		}
		if got[1].DocumentID != "2" || got[1].Content != "chunk-b" || got[1].Complete {
			t.Errorf("second hit = %+v, want doc 2 kept as chunk once the budget is exhausted", got[1])
		}
	})

	t.Run("长度上限非正数时关闭展开", func(t *testing.T) {
		results := []*RetrievalResult{{DocumentID: "1", ChunkIndex: 0, Content: "chunk-0"}}
		fullTexts := map[uint]string{1: "整篇文档正文"}

		got := mergeExpandedResults(results, fullTexts, 0)
		if len(got) != 1 || got[0].Content != "chunk-0" || got[0].Complete {
			t.Errorf("merged = %+v, want the original chunk kept when expansion is disabled", got)
		}
	})

	t.Run("未被整篇文本覆盖的文档原样保留", func(t *testing.T) {
		results := []*RetrievalResult{
			{DocumentID: "1", ChunkIndex: 0, Content: "chunk-0"},
			{DocumentID: "9", ChunkIndex: 3, Content: "not-covered"},
		}
		fullTexts := map[uint]string{1: "整篇文档正文"}

		got := mergeExpandedResults(results, fullTexts, 4000)
		if len(got) != 2 {
			t.Fatalf("merged length = %d, want 2", len(got))
		}
		if got[1].DocumentID != "9" || got[1].ChunkIndex != 3 || got[1].Complete {
			t.Errorf("uncovered hit = %+v, want it preserved as-is", got[1])
		}
	})
}

// TestResultDocumentIDs 覆盖文档 ID 提取：去重且保持首次出现顺序，非数字与零值 ID 跳过。
func TestResultDocumentIDs(t *testing.T) {
	got := resultDocumentIDs([]*RetrievalResult{
		nil,
		{DocumentID: "3"},
		{DocumentID: "abc"},
		{DocumentID: "0"},
		{DocumentID: "3"},
		{DocumentID: "5"},
	})
	if len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Errorf("resultDocumentIDs = %v, want [3 5]", got)
	}
}

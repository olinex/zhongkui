// 本文件覆盖能力模型的纯渲染逻辑：能力集由视图层提供，本包不持有注册表，测试用例均为本地
// 固定夹具，便于验证渲染结果与顺序稳定性。
package capability_test

import (
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
)

// testItems 是按业务域分组、按 ID 排序的固定能力集夹具。
func testItems() []capability.Capability {
	return []capability.Capability{
		{
			ID:          "traffic.list",
			Domain:      "traffic",
			DomainLabel: "流量",
			Intent:      "查询 HTTP 流量列表",
			Method:      "POST",
			Path:        "/api/v1/traffic/http-transport/list",
			Entities:    []string{"http_transports"},
			Access:      capability.AccessRead,
			Invokable:   true,
		},
		{
			ID:          "tag.event.list",
			Domain:      "tag",
			DomainLabel: "标签与事件",
			Intent:      "查看标签事件记录列表",
			Method:      "POST",
			Path:        "/api/v1/tag-event/record/list",
			Entities:    []string{"tag_event_records"},
			Access:      capability.AccessRead,
			Invokable:   true,
		},
	}
}

// TestLookup 校验按能力 ID 查找，未命中返回 false。
func TestLookup(t *testing.T) {
	item, ok := capability.Lookup(testItems(), "tag.event.list")
	if !ok || item.Path != "/api/v1/tag-event/record/list" {
		t.Errorf("Lookup(tag.event.list) = (%+v, %v)", item, ok)
	}
	if _, ok := capability.Lookup(testItems(), "not-exist"); ok {
		t.Errorf("Lookup(not-exist) 不应命中")
	}
	if _, ok := capability.Lookup(nil, "traffic.list"); ok {
		t.Errorf("空能力集不应命中")
	}
}

// TestMapMarkdown 校验能力地图按入参顺序分组渲染、覆盖每条能力，且空能力集返回空串。
func TestMapMarkdown(t *testing.T) {
	items := testItems()
	first := capability.MapMarkdown(items)
	if first != capability.MapMarkdown(items) {
		t.Errorf("能力地图两次生成不一致")
	}
	if !strings.Contains(first, "# 平台能力地图") {
		t.Errorf("能力地图缺少标题: %s", first)
	}
	for _, item := range items {
		if !strings.Contains(first, item.Path) || !strings.Contains(first, item.DomainLabel) {
			t.Errorf("能力地图缺少能力 %q 的路径或业务域", item.ID)
		}
	}
	if strings.Index(first, "## 流量") > strings.Index(first, "## 标签与事件") {
		t.Errorf("能力地图未按入参业务域顺序分组")
	}
	if got := capability.MapMarkdown(nil); got != "" {
		t.Errorf("空能力集的 MapMarkdown = %q, want empty", got)
	}
}

// TestKnowledgeDocs 校验能力分区文档集：一篇整体地图 + 每条能力一篇明细，且逐字节稳定。
func TestKnowledgeDocs(t *testing.T) {
	items := testItems()
	docs := capability.KnowledgeDocs(items)
	if len(docs) != len(items)+1 {
		t.Fatalf("文档数 = %d, want %d", len(docs), len(items)+1)
	}
	if docs[0].Title != "平台能力地图" || docs[0].Content != capability.MapMarkdown(items) {
		t.Errorf("首篇应为整体能力地图")
	}
	for i, item := range items {
		doc := docs[i+1]
		if doc.Title != "平台能力："+item.ID {
			t.Errorf("第 %d 篇标题 = %q", i+1, doc.Title)
		}
		for _, want := range []string{item.Intent, item.Method + " " + item.Path, item.DomainLabel} {
			if !strings.Contains(doc.Content, want) {
				t.Errorf("能力 %q 的明细缺少 %q", item.ID, want)
			}
		}
	}
	if !equalDocs(docs, capability.KnowledgeDocs(items)) {
		t.Errorf("同一能力集两次生成的文档不一致")
	}
}

// equalDocs 比较两份文档集是否逐篇逐字节一致。
func equalDocs(left, right []capability.KnowledgeDoc) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

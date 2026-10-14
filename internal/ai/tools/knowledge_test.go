// 本文件覆盖内置工具 knowledge_search 的纯逻辑：未注入运行上下文、空 query、命中映射、空结果、
// 片段截断与 Info 的入参 schema。全部用例不依赖 PostgreSQL（用假 KnowledgeSearcher 断言收到的
// query），也不依赖 internal/ai（本包不能反向导入）。

package tools

import (
	// 标准库
	"context"
	"encoding/json"
	"strings"
	"testing"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// fakeKnowledgeSearcher 是 KnowledgeSearcher 的测试替身：记录收到的 query 与调用次数，按预设
// 返回 hits 或错误。
type fakeKnowledgeSearcher struct {
	hits  []KnowledgeHit
	err   error
	calls int
	got   []string
}

// Search 记录入参并返回预设结果。
func (s *fakeKnowledgeSearcher) Search(_ context.Context, query string) ([]KnowledgeHit, error) {
	s.calls++
	s.got = append(s.got, query)
	return s.hits, s.err
}

// knowledgeRuntimeCtx 构造一个已注入假检索器与截断长度的 ctx，便于各用例复用。
func knowledgeRuntimeCtx(searcher KnowledgeSearcher, snippetLimit int) context.Context {
	return WithKnowledgeRuntime(context.Background(), KnowledgeRuntime{
		Searcher:     searcher,
		SnippetLimit: snippetLimit,
	})
}

// TestKnowledgeSearch_MissingRuntime 覆盖未注入运行期或检索器为 nil 时返回英文错误。
func TestKnowledgeSearch_MissingRuntime(t *testing.T) {
	run := &knowledgeSearchTool{}
	args := `{"query":"how to write a rule"}`

	if _, err := run.InvokableRun(context.Background(), args); err == nil {
		t.Fatal("no-runtime error = nil, want an English error")
	} else if !strings.Contains(err.Error(), "no knowledge runtime in context") {
		t.Errorf("no-runtime error = %q, want it to mention the missing runtime", err)
	}

	nilSearcher := knowledgeRuntimeCtx(nil, 0)
	if _, err := run.InvokableRun(nilSearcher, args); err == nil {
		t.Fatal("nil-searcher error = nil, want an English error")
	}
}

// TestKnowledgeSearch_EmptyQuery 覆盖 query 为空、全空白或缺失时被拒且不调用检索器。
func TestKnowledgeSearch_EmptyQuery(t *testing.T) {
	cases := map[string]string{
		"empty":      `{"query":""}`,
		"whitespace": `{"query":"   \t\n"}`,
		"missing":    `{}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			s := &fakeKnowledgeSearcher{}
			if _, err := (&knowledgeSearchTool{}).InvokableRun(knowledgeRuntimeCtx(s, 0), args); err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if s.calls != 0 {
				t.Errorf("searcher calls = %d, want 0 (nothing searched)", s.calls)
			}
		})
	}
}

// TestKnowledgeSearch_Hits 覆盖命中映射：返回 JSON 可解出 results 的五个字段，且检索器收到的是
// trim 后的查询串。
func TestKnowledgeSearch_Hits(t *testing.T) {
	s := &fakeKnowledgeSearcher{hits: []KnowledgeHit{
		{DocumentID: "doc-1", Title: "规则编写", ChunkIndex: 2, Content: "先定义主体", Score: 0.875},
		{DocumentID: "doc-2", Title: "部署", ChunkIndex: 0, Content: "启动服务", Score: 0.5},
	}}
	out, err := (&knowledgeSearchTool{}).InvokableRun(knowledgeRuntimeCtx(s, 0), `{"query":"  how to write a rule  "}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if s.calls != 1 {
		t.Fatalf("searcher calls = %d, want 1", s.calls)
	}
	if s.got[0] != "how to write a rule" {
		t.Errorf("searcher query = %q, want the trimmed query", s.got[0])
	}

	var decoded knowledgeSearchResult
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if len(decoded.Results) != 2 {
		t.Fatalf("results length = %d, want 2", len(decoded.Results))
	}
	want := knowledgeSearchItem{DocumentID: "doc-1", Title: "规则编写", ChunkIndex: 2, Content: "先定义主体", Score: 0.875}
	if decoded.Results[0] != want {
		t.Errorf("results[0] = %+v, want %+v", decoded.Results[0], want)
	}
	if decoded.Results[1].DocumentID != "doc-2" || decoded.Results[1].ChunkIndex != 0 {
		t.Errorf("results[1] = %+v, want doc-2 chunk 0", decoded.Results[1])
	}
}

// TestKnowledgeSearch_EmptyResult 覆盖无命中：返回合法 JSON、results 为非 nil 的空数组、message
// 非空且无错误。
func TestKnowledgeSearch_EmptyResult(t *testing.T) {
	s := &fakeKnowledgeSearcher{hits: nil}
	out, err := (&knowledgeSearchTool{}).InvokableRun(knowledgeRuntimeCtx(s, 0), `{"query":"nothing matches"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	if !strings.Contains(out, `"results":[]`) {
		t.Errorf("result %q, want an empty JSON array (not null)", out)
	}
	var decoded knowledgeSearchResult
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if len(decoded.Results) != 0 {
		t.Errorf("results length = %d, want 0", len(decoded.Results))
	}
	if decoded.Message == "" {
		t.Errorf("message is empty, want an empty-result explanation")
	}
}

// TestKnowledgeSearch_SnippetTruncation 覆盖片段截断：SnippetLimit 生效时 content 变短并以 "..."
// 结尾；非正数时不截断。
func TestKnowledgeSearch_SnippetTruncation(t *testing.T) {
	long := "一二三四五六七八九十"
	search := func(limit int) knowledgeSearchResult {
		t.Helper()
		s := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{DocumentID: "d", Content: long}}}
		out, err := (&knowledgeSearchTool{}).InvokableRun(knowledgeRuntimeCtx(s, limit), `{"query":"q"}`)
		if err != nil {
			t.Fatalf("InvokableRun(limit=%d) error = %v", limit, err)
		}
		var decoded knowledgeSearchResult
		if err := json.Unmarshal([]byte(out), &decoded); err != nil {
			t.Fatalf("result %q is not valid JSON: %v", out, err)
		}
		return decoded
	}

	if got := search(3).Results[0].Content; got != "一二三..." {
		t.Errorf("truncated content = %q, want %q", got, "一二三...")
	}
	if got := search(0).Results[0].Content; got != long {
		t.Errorf("untruncated content = %q, want the original %q", got, long)
	}
}

// TestKnowledgeSearch_CompleteNotTruncated 覆盖整篇展开的命中（Complete=true）不受 SnippetLimit
// 截断：整篇已是完整规格，截断只会让模型看不到字段定义。
func TestKnowledgeSearch_CompleteNotTruncated(t *testing.T) {
	long := "一二三四五六七八九十"
	s := &fakeKnowledgeSearcher{hits: []KnowledgeHit{{DocumentID: "d", Content: long, Complete: true}}}
	out, err := (&knowledgeSearchTool{}).InvokableRun(knowledgeRuntimeCtx(s, 3), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun error = %v", err)
	}
	var decoded knowledgeSearchResult
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("result %q is not valid JSON: %v", out, err)
	}
	if got := decoded.Results[0].Content; got != long {
		t.Errorf("complete content = %q, want the untruncated %q", got, long)
	}
}

// TestKnowledgeSearch_Info 覆盖 Info：工具名为 knowledge_search；参数 schema 中 query 存在、类型
// 为 string 且必填；不存在 namespace / docIDs / topK 之类越权入参。
func TestKnowledgeSearch_Info(t *testing.T) {
	info, err := (&knowledgeSearchTool{}).Info(context.Background())
	if err != nil {
		t.Fatalf("Info error = %v", err)
	}
	if info.Name != "knowledge_search" {
		t.Errorf("Info name = %q, want knowledge_search", info.Name)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema error = %v", err)
	}
	query, ok := js.Properties.Get("query")
	if !ok || query.Type != string(schema.String) {
		t.Errorf("query property = %+v (present=%v), want a string", query, ok)
	}
	required := false
	for _, name := range js.Required {
		if name == "query" {
			required = true
		}
	}
	if !required {
		t.Errorf("schema required = %v, want query required", js.Required)
	}
	for _, forbidden := range []string{"namespace", "docIDs", "topK", "top_k", "docs"} {
		if _, ok := js.Properties.Get(forbidden); ok {
			t.Errorf("property %q is present, want only query exposed", forbidden)
		}
	}
}

// TestKnowledgeSearch_BuildResolves 覆盖 init 期登记生效：Build 可按名解析 knowledge_search 并
// 返回可用的 eino 工具。
func TestKnowledgeSearch_BuildResolves(t *testing.T) {
	built, err := Build(context.Background(), "knowledge_search", Deps{})
	if err != nil {
		t.Fatalf("Build(knowledge_search) error = %v", err)
	}
	info, err := built.Info(context.Background())
	if err != nil {
		t.Fatalf("built tool Info error = %v", err)
	}
	if info.Name != "knowledge_search" {
		t.Errorf("built tool name = %q, want knowledge_search", info.Name)
	}
}

// 仅用于确认假替身满足接口（编译期断言）。
var _ KnowledgeSearcher = (*fakeKnowledgeSearcher)(nil)

// 仅用于确认工具满足 eino 的可调用工具接口（编译期断言）。
var _ tool.InvokableTool = (*knowledgeSearchTool)(nil)

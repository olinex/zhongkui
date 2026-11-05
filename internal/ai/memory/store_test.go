package memory

import (
	// 标准库
	"context"
	"strings"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/retrieval"
	"zhongkuisec.com/zhongkui/internal/ai/tools"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// TestNewMemoryStore_NilArgs 测试构造 Store 时的空值校验：db/embed/params 为空返回
// 英文错误，logger 为空允许通过。
func TestNewMemoryStore_NilArgs(t *testing.T) {
	if _, err := NewStore(nil, nil, nil, nil); err == nil {
		t.Error("NewStore(nil, nil, nil, nil) = nil error, want an error")
	}
	if _, err := NewStore(nil, stubEmbedder{}, config.NewParams(nil), nil); err == nil {
		t.Error("NewStore(nil, embed, params, nil) = nil error, want an error")
	}
	if _, err := NewStore(&database.DB{}, nil, config.NewParams(nil), nil); err == nil {
		t.Error("NewStore(db, nil, params, nil) = nil error, want an error")
	}
	if _, err := NewStore(&database.DB{}, stubEmbedder{}, nil, nil); err == nil {
		t.Error("NewStore(db, embed, nil, nil) = nil error, want an error")
	}
	// logger 允许为空
	if _, err := NewStore(&database.DB{}, stubEmbedder{}, config.NewParams(nil), nil); err != nil {
		t.Errorf("NewStore(db, embed, params, nil) error = %v, want nil", err)
	}
}

// stubEmbedder 是满足 embedder 接口的最小桩实现（实现见 aitest.StubEmbedder）。
type stubEmbedder = aitest.StubEmbedder

// TestParseMemoryKind 测试记忆类别解析：空串取缺省 fact，合法取值原样返回，非法取值报英文错误。
func TestParseMemoryKind(t *testing.T) {
	cases := []struct {
		name    string
		in      ai.MemoryKind
		want    ai.MemoryKind
		wantErr bool
	}{
		{name: "空取缺省", in: "", want: ai.MemoryKindFact},
		{name: "合法事实", in: ai.MemoryKindFact, want: ai.MemoryKindFact},
		{name: "合法偏好", in: ai.MemoryKindPreference, want: ai.MemoryKindPreference},
		{name: "合法决策", in: ai.MemoryKindDecision, want: ai.MemoryKindDecision},
		{name: "合法产物", in: ai.MemoryKindArtifact, want: ai.MemoryKindArtifact},
		{name: "非法取值", in: ai.MemoryKind("bogus"), wantErr: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ai.ParseMemoryKind(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("aimodel.ParseMemoryKind(%q) = nil error, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("aimodel.ParseMemoryKind(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("aimodel.ParseMemoryKind(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestValidMemoryStatus 测试记忆状态取值空间校验。
func TestValidMemoryStatus(t *testing.T) {
	if _, err := ai.ParseMemoryStatus(ai.MemoryStatusActive); err != nil {
		t.Errorf("aimodel.ParseMemoryStatus(active) error = %v, want nil", err)
	}
	if _, err := ai.ParseMemoryStatus(ai.MemoryStatusArchived); err != nil {
		t.Errorf("aimodel.ParseMemoryStatus(archived) error = %v, want nil", err)
	}
	if _, err := ai.ParseMemoryStatus(ai.MemoryStatus("bogus")); err == nil {
		t.Error("aimodel.ParseMemoryStatus accepted an undeclared status")
	}
}

// TestNormalizeMemoryContent 测试正文规范化：折叠全部空白并去首尾，且哈希基于规范化正文保持稳定。
func TestNormalizeMemoryContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "折叠空白", in: "  甲\t乙\n\n丙  ", want: "甲 乙 丙"},
		{name: "纯空白归一为空", in: " \t\n ", want: ""},
		{name: "已是规范形", in: "rego 规则生成", want: "rego 规则生成"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeMemoryContent(tt.in); got != tt.want {
				t.Errorf("NormalizeMemoryContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	// 不同排版的同一正文规范化后应一致（HMAC 摘要基于规范化正文，故摘要亦一致）
	a := NormalizeMemoryContent("甲乙  丙\n丁")
	b := NormalizeMemoryContent("甲乙 丙 丁")
	if a != b {
		t.Errorf("normalized bodies of equivalent inputs differ: %q vs %q", a, b)
	}
}

// TestMemoryWrite_ValidationErrors 测试写入的入参校验（db/embed 未就绪即应被拦下，说明校验先于
// 落库与向量化）：空命名空间、空会话、空正文、非法 Agent 类型、非法记忆类别均返回英文错误。
func TestMemoryWrite_ValidationErrors(t *testing.T) {
	store := &Store{}
	cases := []struct {
		name string
		in   tools.MemoryWriteInput
	}{
		{name: "空命名空间", in: tools.MemoryWriteInput{NamespaceName: "  ", SessionID: 1, Content: "内容"}},
		{name: "空会话", in: tools.MemoryWriteInput{NamespaceName: "default", Content: "内容"}},
		{name: "空正文", in: tools.MemoryWriteInput{NamespaceName: "default", SessionID: 1, Content: "   \n"}},
		{name: "非法 Agent 类型", in: tools.MemoryWriteInput{NamespaceName: "default", SessionID: 1, AgentKind: ai.AgentKind("unknown"), Content: "内容"}},
		{name: "非法记忆类别", in: tools.MemoryWriteInput{NamespaceName: "default", SessionID: 1, Kind: ai.MemoryKind("bogus"), Content: "内容"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Write(context.Background(), tt.in)
			if err == nil {
				t.Fatalf("Write() = %+v, want an error", got)
			}
			if got != nil {
				t.Errorf("Write() returned a memory with an error: %+v", got)
			}
		})
	}
}

// TestTruncateBytes_Multibyte 测试按字节截断的 UTF-8 边界安全：不切断中文/emoji，maxBytes<=0
// 或未超限时原样返回。本用例复用包内既有 truncateBytes（同时被知识库工具结果与记忆注入使用）。
func TestTruncateBytes_Multibyte(t *testing.T) {
	if got := retrieval.TruncateBytes("short", 100); got != "short" {
		t.Errorf("retrieval.TruncateBytes(\"short\", 100) = %q, want %q", got, "short")
	}
	if got := retrieval.TruncateBytes("你好世界", 0); got != "你好世界" {
		t.Errorf("retrieval.TruncateBytes(\"你好世界\", 0) = %q, want the input unchanged", got)
	}
	if got := retrieval.TruncateBytes("你好世界", -1); got != "你好世界" {
		t.Errorf("retrieval.TruncateBytes(\"你好世界\", -1) = %q, want the input unchanged", got)
	}
	// 4 字节处落在「好」的中间，必须回退到 3 字节边界，只保留完整的「你」
	if got := retrieval.TruncateBytes("你好世界", 4); got != "你..." {
		t.Errorf("retrieval.TruncateBytes(\"你好世界\", 4) = %q, want %q", got, "你...")
	}
	// emoji 占 4 字节，3 字节处落在其内部，必须回退到 1 字节边界
	if got := retrieval.TruncateBytes("a😀b", 3); got != "a..." {
		t.Errorf("retrieval.TruncateBytes(\"a😀b\", 3) = %q, want %q", got, "a...")
	}
	// 截断结果的前缀不得出现残缺的多字节字符
	if prefix := strings.TrimSuffix(retrieval.TruncateBytes("甲乙丙丁戊己庚辛", 10), "..."); !strings.Contains("甲乙丙丁戊己庚辛", prefix) {
		t.Errorf("truncateBytes prefix %q is not a valid prefix of the input", prefix)
	}
}

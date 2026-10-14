package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// compactSSEEvent 是解析出的单条 SSE 事件：Name 取自 event 行，Data 为 data 行的 JSON 对象。
type compactSSEEvent struct {
	// Name 事件名（compress_start / compress_delta / compress_done）
	Name string
	// Data 事件负载（data 行 JSON 反序列化）
	Data map[string]any
}

// parseCompactSSEEvents 把 SSE 响应体按空行切分为事件块，逐块解析 event 行与 data 行。
// 仅用于测试断言，容忍末尾空块。
func parseCompactSSEEvents(body string) []compactSSEEvent {
	var events []compactSSEEvent
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		ev := compactSSEEvent{Data: map[string]any{}}
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.Data)
			}
		}
		events = append(events, ev)
	}
	return events
}

// newCompactChatServer 启动一个 OpenAI 兼容的假对话端点：stream=true 时按 SSE 返回两段摘要
// 增量后以 [DONE] 收尾（供滚动摘要流式生成），否则返回一段非 JSON 正文（供记忆抽取：解析失败仅
// 记 warning、不触发向量化落库）。端点不发起任何真实模型调用。
func newCompactChatServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			for _, piece := range []string{"压缩", "摘要"} {
				_, _ = io.WriteString(w, `data: {"id":"1","object":"chat.completion.chunk","created":0,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"`+piece+`"},"finish_reason":null}]}`+"\n\n")
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","created":0,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"no-op"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
}

// newCompactingAiRuntime 构造一个可真正完成压缩的 AI 运行时：flash 与 standard 档位绑定到
// baseURL 指向的假对话端点，embedding 档位绑定到不可达地址（装配阶段不联网）。flash 可用使
// 滚动摘要走真实流式路径，便于断言语标推进。
func newCompactingAiRuntime(t *testing.T, db *database.DB, baseURL string) *ai.Runtime {
	t.Helper()
	chat := &modelsai.LlmModel{
		Name: "chat-compact", Kind: llm.ModelKindChat,
		BaseURL: baseURL + "/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "gpt-test", Enabled: true,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("seed compacting chat model: %v", err)
	}
	embed := &modelsai.LlmModel{
		Name: "embed-compact", Kind: llm.ModelKindEmbedding,
		BaseURL: "http://127.0.0.1:1/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "emb-test", Enabled: true,
	}
	if err := db.Create(embed).Error; err != nil {
		t.Fatalf("seed compacting embedding model: %v", err)
	}
	for _, binding := range []modelsai.LlmBinding{
		{Slot: modelsai.SlotFlash, ModelID: chat.ID},
		{Slot: modelsai.SlotStandard, ModelID: chat.ID},
		{Slot: modelsai.SlotEmbedding, ModelID: embed.ID},
	} {
		if err := db.Omit("Model").Create(&binding).Error; err != nil {
			t.Fatalf("seed slot binding %q: %v", binding.Slot, err)
		}
	}
	rt, err := ai.NewRuntime(context.Background(), db, nil, newKnowledgeTestLogger())
	if err != nil {
		t.Fatalf("build compacting ai runtime: %v", err)
	}
	return rt
}

// newSessionCompactTestRouter 创建仅注册会话压缩路由的测试引擎（装配范式同 newSessionTestRouter）。
func newSessionCompactTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newKnowledgeTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Set(middlewares.AiRuntimeCtxName, rt)
		user := &core.User{}
		user.ID = sessionTestUserID
		c.Set(middlewares.UserCtxName, user)
		c.Next()
	})
	group := r.Group("/ai/session")
	group.POST("/:id/compact", AgentSessionCompact)
	return r
}

// compactPath 拼接会话压缩路径。
func compactPath(id uint) string {
	return "/ai/session/" + strconv.FormatUint(uint64(id), 10) + "/compact"
}

// loadSessionMessages 读取一个会话的全部消息（按 seq 升序），供测试定位折叠边界。
func loadSessionMessages(t *testing.T, db *database.DB, sessionID uint) []*modelsai.AgentMessage {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	messages, err := store.ListMessagesAfter(context.Background(), sessionID, sessionTestUserID, 0)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}
	return messages
}

// reloadCompactSession 重新读取一个会话行，供断言游标与摘要落库。
func reloadCompactSession(t *testing.T, db *database.DB, sessionID uint) *modelsai.AgentSession {
	t.Helper()
	row := &modelsai.AgentSession{}
	if err := db.Where("id = ?", sessionID).First(row).Error; err != nil {
		t.Fatalf("reload session %d: %v", sessionID, err)
	}
	return row
}

// TestAgentSessionCompact 会话压缩的 SSE 语义：成功时按 start → delta → done 流式下发并推进
// 滚动摘要游标；userMessageIndex 指定折叠边界时仅折叠该轮及更早消息、更晚消息不受影响；序号越界
// 沿用本地化 404（非 SSE）；flash 档位未绑定时仍以 start → done(foldedCount=0) 收敛。
func TestAgentSessionCompact(t *testing.T) {
	Convey("压缩 AI 会话上下文（SSE 流式）", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})

		Convey("全量折叠成功时下发 start→delta→done 且游标推进到末条消息", func() {
			srv := newCompactChatServer(t)
			defer srv.Close()
			rt := newCompactingAiRuntime(t, db, srv.URL)
			r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
			src := seedBranchSourceSession(t, db, "源会话")
			messages := loadSessionMessages(t, db, src.ID)
			So(len(messages), ShouldEqual, 4)
			expectedUpto := messages[len(messages)-1].Seq

			w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(src.ID), map[string]any{})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get("Content-Type"), ShouldContainSubstring, "text/event-stream")

			events := parseCompactSSEEvents(w.Body.String())
			So(len(events), ShouldBeGreaterThanOrEqualTo, 2)
			So(events[0].Name, ShouldEqual, "compress_start")
			So(events[0].Data["reason"], ShouldEqual, "manual")
			hasDelta := false
			for _, e := range events {
				if e.Name == "compress_delta" {
					hasDelta = true
				}
			}
			So(hasDelta, ShouldBeTrue)
			last := events[len(events)-1]
			So(last.Name, ShouldEqual, "compress_done")
			So(last.Data["foldedCount"], ShouldEqual, float64(4))
			So(last.Data["summaryUptoSeq"], ShouldEqual, float64(expectedUpto))

			row := reloadCompactSession(t, db, src.ID)
			So(row.SummaryUptoSeq, ShouldEqual, uint(expectedUpto))
			So(string(row.Summary.Value()), ShouldNotBeEmpty)
		})

		Convey("userMessageIndex=1 只折叠第 1 轮，更晚消息与游标不受影响", func() {
			srv := newCompactChatServer(t)
			defer srv.Close()
			rt := newCompactingAiRuntime(t, db, srv.URL)
			r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
			src := seedBranchSourceSession(t, db, "源会话")
			messages := loadSessionMessages(t, db, src.ID)
			So(len(messages), ShouldEqual, 4)
			// 第 1 条用户消息（u1）之后的首条助手消息（a1）的 seq 即折叠上界。
			firstAssistantSeq := messages[1].Seq

			w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(src.ID), map[string]any{
				"userMessageIndex": 1,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			events := parseCompactSSEEvents(w.Body.String())
			last := events[len(events)-1]
			So(last.Name, ShouldEqual, "compress_done")
			So(last.Data["foldedCount"], ShouldEqual, float64(2))
			So(last.Data["summaryUptoSeq"], ShouldEqual, float64(firstAssistantSeq))

			// 游标停在第 1 轮助手消息：更晚的第 2 轮（u2/a2）未被折叠（seq 均 > 游标）。
			row := reloadCompactSession(t, db, src.ID)
			So(row.SummaryUptoSeq, ShouldEqual, uint(firstAssistantSeq))
			later := loadMessagesAfter(t, db, src.ID, uint(firstAssistantSeq))
			So(len(later), ShouldEqual, 2)
			So(later[0].Seq, ShouldBeGreaterThan, uint64(row.SummaryUptoSeq))
		})

		Convey("userMessageIndex 越界返回本地化 404（非 SSE）", func() {
			rt := newEnabledAiRuntime(t, db)
			r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
			src := seedBranchSourceSession(t, db, "源会话")

			w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(src.ID), map[string]any{
				"userMessageIndex": 999,
			})
			So(w.Code, ShouldEqual, http.StatusNotFound)
			So(w.Header().Get("Content-Type"), ShouldContainSubstring, "application/json")
			So(w.Body.String(), ShouldContainSubstring, "not found")
		})

		Convey("flash 档位未绑定时仍以 start→done(foldedCount=0) 收敛（SSE 格式）", func() {
			rt := newEnabledAiRuntime(t, db)
			r := newSessionCompactTestRouter(db, knowledgeTestNamespace, rt)
			src := seedBranchSourceSession(t, db, "源会话")

			w := doKnowledgeJSONRequest(r, http.MethodPost, compactPath(src.ID), map[string]any{})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get("Content-Type"), ShouldContainSubstring, "text/event-stream")

			events := parseCompactSSEEvents(w.Body.String())
			So(len(events), ShouldEqual, 2)
			So(events[0].Name, ShouldEqual, "compress_start")
			So(events[0].Data["reason"], ShouldEqual, "manual")
			So(events[1].Name, ShouldEqual, "compress_done")
			// 失败路径折叠条数为 0，因 omitempty 被省略（不是显式 0）。
			So(events[1].Data["foldedCount"], ShouldBeNil)

			// 摘要未生成时游标不动（仍为 0）。
			row := reloadCompactSession(t, db, src.ID)
			So(row.SummaryUptoSeq, ShouldEqual, uint(0))
		})
	})
}

// loadMessagesAfter 读取 seq > afterSeq 的消息（按 seq 升序），供边界断言。
func loadMessagesAfter(t *testing.T, db *database.DB, sessionID uint, afterSeq uint) []*modelsai.AgentMessage {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	messages, err := store.ListMessagesAfter(context.Background(), sessionID, sessionTestUserID, afterSeq)
	if err != nil {
		t.Fatalf("ListMessagesAfter error = %v", err)
	}
	return messages
}

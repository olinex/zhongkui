package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

// newCompactingAiRuntime 播种 chat/embedding 模型与档位绑定，并用指定 URL
// 构建 AI 运行时；用于提示词优化等需要真实假端点的用例。
func newCompactingAiRuntime(t *testing.T, db *database.DB, chatURL string) *ai.Runtime {
	t.Helper()
	chat := &modelsai.LlmModel{
		Name: "chat-test", Kind: llm.ModelKindChat,
		BaseURL: chatURL, APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "gpt-test", Enabled: true, APIMode: llm.APIModeChat,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("seed chat model: %v", err)
	}
	embed := &modelsai.LlmModel{
		Name: "embed-test", Kind: llm.ModelKindEmbedding,
		BaseURL: chatURL, APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "emb-test", Enabled: true,
	}
	if err := db.Create(embed).Error; err != nil {
		t.Fatalf("seed embedding model: %v", err)
	}
	for _, binding := range []modelsai.LlmBinding{
		{Slot: modelsai.SlotStandard, ModelID: chat.ID},
		{Slot: modelsai.SlotFlash, ModelID: chat.ID},
		{Slot: modelsai.SlotEmbedding, ModelID: embed.ID},
	} {
		if err := db.Omit("Model").Create(&binding).Error; err != nil {
			t.Fatalf("seed slot binding %q: %v", binding.Slot, err)
		}
	}
	rt, err := ai.NewRuntime(context.Background(), db, nil, newKnowledgeTestLogger())
	if err != nil {
		t.Fatalf("build ai runtime: %v", err)
	}
	return rt
}

// newPromptOptimizeChatServer 启动一个 OpenAI 兼容的假对话端点：固定返回一段优化后的提问正文，
// 不发起任何真实模型调用。
func newPromptOptimizeChatServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","object":"chat.completion","created":0,"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":`+mustJSON(t, reply)+`},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
}

// mustJSON 把字符串编码为 JSON 字面量（含引号转义），供拼接假响应正文。
func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal fake reply: %v", err)
	}
	return string(b)
}

// newPromptOptimizeTestRouter 创建仅注册提示词优化路由的测试引擎（装配范式同
// newSessionCompactTestRouter）。
func newPromptOptimizeTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group := r.Group("/ai")
	group.POST("/prompt/optimize", AgentPromptOptimize)
	return r
}

// TestAgentPromptOptimize 提示词优化端点语义：AI 已启用且诉求非空时经 flash 档位返回优化后的提问
// 文本；诉求裁剪为空返回 400；AI 未启用时按既有语义返回本地化 400 且不进入模型调用。
func TestAgentPromptOptimize(t *testing.T) {
	Convey("优化用户提问的端点", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})

		Convey("成功时返回优化后的提问文本", func() {
			srv := newPromptOptimizeChatServer(t, "请给出优化后的提问")
			defer srv.Close()
			rt := newCompactingAiRuntime(t, db, srv.URL)
			r := newPromptOptimizeTestRouter(db, knowledgeTestNamespace, rt)

			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/prompt/optimize", map[string]any{"query": "帮我看看"})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *AgentPromptOptimizeOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Query, ShouldEqual, "请给出优化后的提问")
		})

		Convey("空诉求返回 400", func() {
			rt := newEnabledAiRuntime(t, db)
			r := newPromptOptimizeTestRouter(db, knowledgeTestNamespace, rt)

			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/prompt/optimize", map[string]any{"query": "   "})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "query")
		})

		Convey("AI 未启用返回本地化 400", func() {
			rt := newDisabledAiRuntime(t, db)
			r := newPromptOptimizeTestRouter(db, knowledgeTestNamespace, rt)

			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/prompt/optimize", map[string]any{"query": "帮我看看"})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(w.Body.String(), ShouldContainSubstring, "ai feature is disabled")
		})
	})
}

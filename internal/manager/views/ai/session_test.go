package ai

import (
	// 标准库
	"context"
	"encoding/json"
	"net/http"
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
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// sessionTestUserID 是 AI 会话视图测试使用的属主用户 ID（写入会话的 user_id 并作为请求用户）。
const sessionTestUserID = uint(1)

// newSessionTestRouter 创建 AI 会话测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时。
func newSessionTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group.POST("/create", AgentSessionCreate)
	group.POST("/:id/branch", AgentSessionBranch)
	return r
}

// seedBranchSourceSession 在测试库中播种一个源会话（标准档位 / knowledge 类型）并写入 2 轮问答
// （u1/a1/u2/a2），返回源会话；供会话分支用例使用。
func seedBranchSourceSession(t *testing.T, db *database.DB, title string) *modelsai.AgentSession {
	t.Helper()
	store, err := ai.NewSessionStore(db)
	if err != nil {
		t.Fatalf("NewSessionStore error = %v", err)
	}
	src, err := store.CreateSession(context.Background(), knowledgeTestNamespace, title, modelsai.SlotStandard, modelsai.AgentKindKnowledge, sessionTestUserID)
	if err != nil {
		t.Fatalf("CreateSession error = %v", err)
	}
	messages := []struct {
		role    modelsai.MessageRole
		content string
	}{
		{modelsai.MessageRoleUser, "u1"},
		{modelsai.MessageRoleAssistant, "a1"},
		{modelsai.MessageRoleUser, "u2"},
		{modelsai.MessageRoleAssistant, "a2"},
	}
	for _, m := range messages {
		if _, err := store.AppendMessage(context.Background(), &modelsai.AgentMessage{
			SessionID:     src.ID,
			NamespaceName: knowledgeTestNamespace,
			UserID:        sessionTestUserID,
			Role:          m.role,
			Content:       serializer.NewCrypto([]byte(m.content)),
		}); err != nil {
			t.Fatalf("AppendMessage error = %v", err)
		}
	}
	return src
}

// TestNewAgentMessageItemOutBlocks 校验消息列表输出项携带有序内容块：按 block_seq 升序透传五种 kind 的
// 载荷（与块表一致），且旧出参字段 reasoning / questionnaires 已从出参移除（序列化后无对应顶层 JSON 键）。
func TestNewAgentMessageItemOutBlocks(t *testing.T) {
	Convey("消息列表输出项携带有序内容块并移除旧出参", t, func() {
		order := []modelsai.AgentMessageBlockKind{
			modelsai.AgentMessageBlockKindReasoning,
			modelsai.AgentMessageBlockKindToolCalls,
			modelsai.AgentMessageBlockKindApproval,
			modelsai.AgentMessageBlockKindQuestionnaire,
			modelsai.AgentMessageBlockKindContent,
		}
		payloads := map[modelsai.AgentMessageBlockKind]string{
			modelsai.AgentMessageBlockKindReasoning:     `{"text":"思考"}`,
			modelsai.AgentMessageBlockKindToolCalls:     `{"calls":[{"callID":"c1","toolName":"search"}]}`,
			modelsai.AgentMessageBlockKindApproval:      `{"calls":[{"callID":"c2","toolName":"api_invoke"}]}`,
			modelsai.AgentMessageBlockKindQuestionnaire: `{"questions":[{"id":"q1","question":"要查什么？"}],"awaiting":true}`,
			modelsai.AgentMessageBlockKindContent:       `{"text":"正文"}`,
		}
		blocks := make([]*modelsai.AgentMessageBlock, 0, len(order))
		for i, kind := range order {
			blocks = append(blocks, &modelsai.AgentMessageBlock{
				BlockSeq: uint64(i + 1),
				Kind:     kind,
				Payload:  serializer.NewCrypto([]byte(payloads[kind])),
			})
		}

		Convey("块按 block_seq 升序透传且载荷与块表一致", func() {
			out := newAgentMessageItemOut(&modelsai.AgentMessage{
				Role:    modelsai.MessageRoleAssistant,
				Content: serializer.NewCrypto([]byte("答案")),
			}, blocks)
			So(len(out.Blocks), ShouldEqual, len(order))
			for i, b := range out.Blocks {
				So(b.Seq, ShouldEqual, uint64(i+1))
				So(b.Kind, ShouldEqual, order[i])
				So(string(b.Payload), ShouldEqual, payloads[order[i]])
			}
		})

		Convey("旧出参字段 reasoning 与 questionnaires 已移除", func() {
			out := newAgentMessageItemOut(&modelsai.AgentMessage{
				Role:    modelsai.MessageRoleAssistant,
				Content: serializer.NewCrypto([]byte("答案")),
			}, blocks)
			raw, err := json.Marshal(out)
			So(err, ShouldBeNil)
			var decoded map[string]json.RawMessage
			So(json.Unmarshal(raw, &decoded), ShouldBeNil)
			_, hasReasoning := decoded["reasoning"]
			_, hasQuestionnaires := decoded["questionnaires"]
			So(hasReasoning, ShouldBeFalse)
			So(hasQuestionnaires, ShouldBeFalse)
			_, hasBlocks := decoded["blocks"]
			So(hasBlocks, ShouldBeTrue)
		})

		Convey("无块时省略 blocks", func() {
			out := newAgentMessageItemOut(&modelsai.AgentMessage{
				Role:    modelsai.MessageRoleUser,
				Content: serializer.NewCrypto([]byte("提问")),
			}, nil)
			So(out.Blocks, ShouldBeNil)
			raw, err := json.Marshal(out)
			So(err, ShouldBeNil)
			So(strings.Contains(string(raw), "blocks"), ShouldBeFalse)
		})
	})
}

// TestAgentSessionCreate 创建会话的输入语义：请求携带首个用户问题（query），会话主题由后端生成
// （flash 档位未绑定时回退为按字节截断的问题文本）并随响应回传、落库；缺失 query 时 400 且不落库。
func TestAgentSessionCreate(t *testing.T) {
	Convey("创建 AI 会话据首个问题生成主题", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		// 启用运行时只绑定 standard 与 embedding 档位：flash 档位未绑定，主题生成本次走兜底路径
		rt := newEnabledAiRuntime(t, db)
		r := newSessionTestRouter(db, knowledgeTestNamespace, rt)

		Convey("首条问题生成会话主题并落库", func() {
			query := "如何配置标签规则"
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/session/create", map[string]any{
				"query": query,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                  `json:"code"`
				Data *AgentSessionItemOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Title, ShouldEqual, query)
			So(resp.Data.AgentKind, ShouldEqual, modelsai.AgentKindKnowledge)
			So(resp.Data.ID, ShouldBeGreaterThan, 0)

			var row modelsai.AgentSession
			So(db.Where("id = ?", resp.Data.ID).First(&row).Error, ShouldBeNil)
			So(row.Title, ShouldEqual, query)
			So(row.NamespaceName, ShouldEqual, knowledgeTestNamespace)
		})

		Convey("缺失首个问题被拒绝且不落库", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/session/create", map[string]any{})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&modelsai.AgentSession{}).
				Where("namespace_name = ?", knowledgeTestNamespace).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("超长首个问题的兜底主题按字节截断", func() {
			query := strings.Repeat("a", 100)
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/session/create", map[string]any{
				"query": query,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *AgentSessionItemOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Title, ShouldEqual, strings.Repeat("a", 64)+"...")
		})
	})
}

// TestAgentSessionBranch 会话分支的语义：以源会话第 1 轮用户消息为结尾分支时，新会话仅复制该轮
// 及其之前的消息（u1/a1），标题缺省沿用源会话，源会话消息不受影响。
func TestAgentSessionBranch(t *testing.T) {
	Convey("以第 1 轮用户消息为结尾分支会话", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rt := newEnabledAiRuntime(t, db)
		r := newSessionTestRouter(db, knowledgeTestNamespace, rt)
		src := seedBranchSourceSession(t, db, "源会话")

		w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/session/"+strconv.FormatUint(uint64(src.ID), 10)+"/branch", map[string]any{
			"userMessageIndex": 1,
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int                  `json:"code"`
			Data *AgentSessionItemOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data, ShouldNotBeNil)
		So(resp.Data.ID, ShouldNotEqual, src.ID)
		So(resp.Data.Title, ShouldEqual, "源会话")
		So(resp.Data.Slot, ShouldEqual, modelsai.SlotStandard)
		So(resp.Data.AgentKind, ShouldEqual, modelsai.AgentKindKnowledge)

		store, err := ai.NewSessionStore(db)
		So(err, ShouldBeNil)
		branched, err := store.ListMessages(context.Background(), resp.Data.ID, sessionTestUserID, 0, 0)
		So(err, ShouldBeNil)
		So(len(branched), ShouldEqual, 2)
		So(string(branched[0].Content.Value()), ShouldEqual, "u1")
		So(string(branched[1].Content.Value()), ShouldEqual, "a1")

		// 源会话消息不受分支影响。
		source, err := store.ListMessages(context.Background(), src.ID, sessionTestUserID, 0, 0)
		So(err, ShouldBeNil)
		So(len(source), ShouldEqual, 4)
	})
}

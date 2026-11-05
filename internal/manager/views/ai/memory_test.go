package ai

import (
	// 标准库
	"encoding/json"
	"net/http"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// newMemoryTestRouter 创建长期记忆视图测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时（需为启用态，requireAiEnabled 才放行）。
func newMemoryTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	r.POST("/ai/memory/list", AgentMemoryListGet)
	return r
}

// seedMemorySession 插入一条 agent_sessions 行并返回会话 ID（记忆的 session_id 外键依赖）。
func seedMemorySession(t *testing.T, db *database.DB, namespace string, userID uint) uint {
	t.Helper()
	var sessionID uint
	if err := db.Raw(
		`INSERT INTO agent_sessions (namespace_name, user_id, title, target_model, status, agent_kind, review_valve)
		 VALUES (?, ?, 'memory-list-test', '', 'active', 'knowledge', 'each') RETURNING id`,
		namespace, userID,
	).Scan(&sessionID).Error; err != nil {
		t.Fatalf("seed memory session: %v", err)
	}
	return sessionID
}

// seedMemory 以原始 SQL 插入一条长期记忆并返回其 ID：正文留空（AES-GCM 最小密文长度以下按空处理），
// search 列按「标题 + 正文」构造 tsvector，与生产写入语义一致。
func seedMemory(t *testing.T, db *database.DB, namespace string, userID, sessionID uint, title, text string) uint {
	t.Helper()
	var id uint
	if err := db.Raw(
		`INSERT INTO agent_memories
		 (namespace_name, user_id, agent_kind, session_id, kind, title, content, embedding, hash, status, search)
		 VALUES (?, ?, 'knowledge', ?, 'fact', ?, '\x'::bytea,
		         ('[' || repeat('0,', 1535) || '0]')::vector, repeat(md5(random()::text), 2), 'active',
		         to_tsvector('simple', ?))
		 RETURNING id`,
		namespace, userID, sessionID, title, text,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed memory %q: %v", title, err)
	}
	return id
}

// TestAgentMemoryListSearch 覆盖长期记忆列表复用统一搜索基座后的行为：按当前用户隔离、
// keyword 走 search tsvector 全文检索、未命中返回空列表。
func TestAgentMemoryListSearch(t *testing.T) {
	Convey("长期记忆列表走统一搜索基座", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newMemoryTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		// 第二位用户的记忆用于验证列表的属主隔离
		otherUserID := uint(2)
		if err := db.Exec(
			`INSERT INTO users (id, union_id, nickle_name, username, password, email)
			 VALUES (?, '00000000-0000-0000-0000-000000000104', 'memory-view-test', 'memory-view-test-user', '\x00'::bytea, 'memory-view-test@example.com')
			 ON CONFLICT DO NOTHING`,
			otherUserID,
		).Error; err != nil {
			t.Fatalf("seed other user: %v", err)
		}
		sessionID := seedMemorySession(t, db, knowledgeTestNamespace, sessionTestUserID)
		seedMemory(t, db, knowledgeTestNamespace, sessionTestUserID, sessionID, "alpha plan", "alpha beta content")
		seedMemory(t, db, knowledgeTestNamespace, sessionTestUserID, sessionID, "gamma note", "gamma delta content")
		seedMemory(t, db, knowledgeTestNamespace, otherUserID, sessionID, "alpha other", "alpha beta content")

		list := func(filterBy map[string]any) *projectedListResponse {
			body := map[string]any{
				"greedy":   false,
				"page":     1,
				"pageSize": 10,
				"orderBy":  []map[string]any{{"name": "updatedAt", "order": -1}},
			}
			if filterBy != nil {
				body["filterBy"] = filterBy
			}
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/memory/list", body)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp projectedListResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			return &resp
		}

		all := list(nil)
		So(all.Data.Total, ShouldEqual, 2)
		So(all.Data.Items, ShouldHaveLength, 2)

		hit := list(map[string]any{
			"keyword": map[string]any{"action": "fts", "value": "alpha"},
		})
		So(hit.Data.Total, ShouldEqual, 1)
		So(hit.Data.Items, ShouldHaveLength, 1)
		So(hit.Data.Items[0]["title"], ShouldEqual, "alpha plan")

		miss := list(map[string]any{
			"keyword": map[string]any{"action": "fts", "value": "missing"},
		})
		So(miss.Data.Total, ShouldEqual, 0)
		So(miss.Data.Items, ShouldBeEmpty)
	})
}

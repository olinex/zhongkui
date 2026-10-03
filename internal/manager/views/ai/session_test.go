package ai

import (
	// 标准库
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	modelsai "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// newSessionTestRouter 创建 AI 会话测试路由引擎，注入 i18n、日志、核心库、命名空间与 AI 运行时。
func newSessionTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newKnowledgeTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Set(middlewares.AiRuntimeCtxName, rt)
		c.Next()
	})
	group := r.Group("/ai/session")
	group.POST("/create", AgentSessionCreate)
	return r
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

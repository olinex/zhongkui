// 本文件承载「原生接口知识目录」的离线装配：以空操作中间件构造最小注册上下文，调用与在线
// 路由注册（RouteRegister）同一份域注册清单，从而在无 DB / Redis / Etcd 的环境下取得与生产一致
// 的接口目录快照，供 zki sync apis 与守卫测试消费。
package manager

import (
	// 标准库
	"net/http"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/manager/views/admin"
	"zhongkuisec.com/zhongkui/internal/manager/views/ai"
	"zhongkuisec.com/zhongkui/internal/manager/views/datasource"
	"zhongkuisec.com/zhongkui/internal/manager/views/etl"
	"zhongkuisec.com/zhongkui/internal/manager/views/llm"
	"zhongkuisec.com/zhongkui/internal/manager/views/notification"
	"zhongkuisec.com/zhongkui/internal/manager/views/plaintext"
	"zhongkuisec.com/zhongkui/internal/manager/views/tag"
	"zhongkuisec.com/zhongkui/internal/manager/views/traffic"
	"zhongkuisec.com/zhongkui/internal/manager/views/user"
)

// noopMiddleware 是离线装配用的空操作中间件：注册期不做任何处理，仅占位以满足分组中间件参数。
func noopMiddleware(c *gin.Context) {}

// newOfflineRegisterContext 构造离线路由注册上下文：真实 gin 引擎 + /api/v1 组 + 空操作中间件。
// 不注入任何依赖实例（Injector 为 nil），仅用于收集路由注册（处理器捕获的依赖在注册期不被调用）。
func newOfflineRegisterContext() (*gin.Engine, *views.RegisterContext) {
	// 离线装配只借用 gin 路由表枚举接口目录，不对外提供服务；路由调试输出（[GIN-debug] 逐条
	// 清单）在 CLI 中会盖过 sync 自身的进度与明细输出，故强制置为发布模式默认不打印。
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	rc := &views.RegisterContext{
		Engine:               engine,
		V1Group:              engine.Group("/api/v1"),
		Authenticator:        noopMiddleware,
		ForceAuthenticate:    noopMiddleware,
		Policy:               noopMiddleware,
		Authorize:            noopMiddleware,
		CoreDatabase:         noopMiddleware,
		RawDatabase:          noopMiddleware,
		S3:                   noopMiddleware,
		ObjectCache:          noopMiddleware,
		Etcd:                 noopMiddleware,
		Namespace:            noopMiddleware,
		PipelineInspector:    noopMiddleware,
		PipelineClient:       noopMiddleware,
		TagRuleEngine:        noopMiddleware,
		FeatureRegistry:      noopMiddleware,
		PasswordResetLimiter: noopMiddleware,
		MessagePool:          noopMiddleware,
		SQLiteVTabPools:      noopMiddleware,
		Ai:                   noopMiddleware,
	}
	return engine, rc
}

// registerViewRoutes 注册全局功能开关端点与全部视图域路由。在线装配（RouteRegister）与离线目录
// 装配（CollectApiCatalog）必须共用本函数，新增/移除视图域只改此处，避免两份清单漂移。
func registerViewRoutes(rc *views.RegisterContext) {
	// 全局功能开关列表（公共接口，无需认证）：仅输出，无请求体。
	rc.V1Group.POST("/features/list", rc.FeatureRegistry, views.FeaturesListGet)
	views.RegisterApi(rc.V1Group, "/features/list", "查询全部功能开关（key + 是否启用）",
		views.WithMethod(http.MethodPost, views.WithBodyOutput[views.FeaturesListGetOut]()),
	)

	admin.RegisterRoutes(rc)
	traffic.RegisterRoutes(rc)
	tag.RegisterRoutes(rc)
	datasource.RegisterRoutes(rc)
	plaintext.RegisterRoutes(rc)
	user.RegisterRoutes(rc)
	notification.RegisterRoutes(rc)
	etl.RegisterRoutes(rc)
	ai.RegisterRoutes(rc)
	llm.RegisterRoutes(rc)
}

// CollectApiCatalog 离线装配全部视图域并返回原生接口目录快照（无需 DB / Redis / Etcd）。
func CollectApiCatalog() []views.ApiDescriptor {
	_, rc := newOfflineRegisterContext()
	registerViewRoutes(rc)
	return views.ApiCatalog()
}

// CollectApiKnowledgeDocs 离线装配全部视图域并返回纳入 RAG 的原生接口知识文档。
func CollectApiKnowledgeDocs() []views.ApiKnowledgeDoc {
	_, rc := newOfflineRegisterContext()
	registerViewRoutes(rc)
	return views.ApiKnowledgeDocs()
}

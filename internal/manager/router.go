// Package manager 提供 API 管理层的核心能力：HTTP 服务的启动与生命周期编排、配置管理、JWT 认证
// 与 RBAC 授权中间件，以及把路由分发给各业务子包的 RegisterRoutes。
//
// Router 相关逻辑：HTTP 路由注册的薄层。
//
// 本文件承载全局中间件装配、公共端点（/ping、/features/list）注册，
// 以及把路由注册分发给各业务子包的 RegisterRoutes（见 internal/manager/views/*/register.go）。
// 原 internal/manager/router 子目录已删除，此处并入 package manager。
package manager

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	datasourcepkg "zhongkui.com/zhongkui/internal/datasource"
	"zhongkui.com/zhongkui/internal/manager/config"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/manager/views/admin"
	"zhongkui.com/zhongkui/internal/manager/views/ai"
	"zhongkui.com/zhongkui/internal/manager/views/datasource"
	"zhongkui.com/zhongkui/internal/manager/views/etl"
	"zhongkui.com/zhongkui/internal/manager/views/llm"
	"zhongkui.com/zhongkui/internal/manager/views/notification"
	"zhongkui.com/zhongkui/internal/manager/views/plaintext"
	"zhongkui.com/zhongkui/internal/manager/views/tag"
	"zhongkui.com/zhongkui/internal/manager/views/traffic"
	"zhongkui.com/zhongkui/internal/manager/views/user"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// RouteRegister 注册全部 HTTP 路由：初始化全局中间件链后，
// 按业务域委托各子包的 RegisterRoutes 完成路由注册。
func RouteRegister(ctx context.Context, i do.Injector) error {
	viper := provider.MustInvokeViper(i)
	kd := provider.MustInvokeKeyDerivation(i)
	etcd := provider.MustInvokeEtcdClient(i)
	coreDB := provider.MustInvokeCoreDatabase(i)
	rawDB := provider.MustInvokeRawDatabase(i)
	s3Client := provider.MustInvokeS3(i)
	objCache := provider.MustInvokeObjectCache(i)

	// 初始化命名空间缓存：设置数据库引用并预加载全量命名空间
	core.Cache.SetDB(coreDB)
	if err := core.Cache.Load(ctx); err != nil {
		return errors.WithStack(err)
	}

	// 启动重载标签事件第三方数据源：扫描已启用数据源注册为 SQLite vtable，
	// 使前端动态配置在重启后恢复注册状态（rego sqlite.query 立即可查）
	sqliteVTabPools := provider.MustInvokeSQLiteVTabPools(i)
	if err := datasourcepkg.Reload(ctx, coreDB, sqliteVTabPools); err != nil {
		return err
	}

	engine := provider.MustInvokeGin(i)
	inspector := provider.MustInvokePipelineInspector(i)
	pipelineClient := provider.MustInvokePipelineClient(i)
	apiLogger := provider.MustInvokeApiLogger(i)
	rbac := provider.MustInvokeRbac(i)
	tagEngine := provider.MustInvokeTagEngine(i)
	featureRegistry := provider.MustInvokeFeatureRegistry(i)
	messagePool := provider.MustInvokeMessagePool(i)
	aiRuntime := provider.MustInvokeAi(ctx, i)
	cfg, err := config.ExtractConfig(viper, validator.Validate)
	if err != nil {
		return errors.WithStack(err)
	}

	// 创建JWT认证器，用于用户身份验证和令牌管理
	authenticator := middlewares.NewJwtAuthenticator(&cfg.Auth, coreDB, kd)

	// ==================== 全局中间件配置 ====================
	// 以下中间件将应用于所有HTTP请求，按照添加顺序执行

	// gin.Recovery() - Gin框架的崩溃恢复中间件，防止未处理的panic导致服务崩溃
	engine.Use(gin.Recovery())
	// OtelMiddleware - OTEL中间件，用于记录请求跟踪和指标
	engine.Use(middlewares.OtelMiddleware())
	// ConfigMiddleware - 配置中间件，将应用程序配置注入到请求上下文
	engine.Use(config.ConfigMiddleware(cfg))
	// SecureHeadersMiddleware - 安全响应头中间件，设置CSP、HSTS等安全头
	engine.Use(middlewares.SecureHeadersMiddleware(&cfg.Secure))
	// CORSHeadersMiddleware - CORS跨域中间件，处理跨域请求
	engine.Use(middlewares.CORSHeadersMiddleware(&cfg.Secure.CORS))
	// ZapLoggerMiddleware - 结构化日志中间件，记录请求详情和性能指标
	engine.Use(middlewares.ZapLoggerMiddleware(apiLogger))
	// TranslateMiddleware - 国际化中间件，支持多语言错误消息和界面
	engine.Use(middlewares.TranslateMiddleware())

	// ==================== 分组中间件配置 ====================
	// 以下中间件将根据路由分组选择性应用，提供细粒度的访问控制和功能支持

	// Authenticator - 认证器中间件，将认证器实例注入请求上下文
	authenticatorMiddleware := authenticator.AuthenticatorMiddleware()
	// ForceAuthenticate - 强制认证中间件，要求请求必须通过JWT认证
	forceAuthenticateMiddleware := authenticator.AuthenticateMiddleware(false)
	// Policy - RBAC策略中间件，将策略引擎注入请求上下文
	policyMiddleware := middlewares.RbacPolicyMiddleware(rbac)
	// Authorize - 权限验证中间件：基于请求路径（api:{FullPath}）+ HTTP 方法
	// 动态授权，权限点由 rbac_policy_templates 模板校验合法范围、用户组策略生效
	authorizeMiddleware := middlewares.RbacPolicyAuthorizeMiddleware()
	// CoreDatabase - 数据库中间件，将核心数据库连接注入请求上下文
	coreDatabaseMiddleware := middlewares.CoreDatabaseMiddleware(coreDB)
	// RawDatabase - raw 库中间件，供标签事件规则 dry-run 查询 tag_summings
	rawDatabaseMiddleware := middlewares.RawDatabaseMiddleware(rawDB)
	// S3 - S3 客户端中间件，供原始流量 body 明文读取
	s3Middleware := middlewares.S3ClientMiddleware(s3Client)
	// ObjectCache - S3 通用加密对象缓存中间件，供知识库原件上传/下载/清理使用
	objectCacheMiddleware := middlewares.ObjectCacheMiddleware(objCache)
	// Etcd - etcd客户端中间件，将etcd客户端注入请求上下文
	etcdMiddleware := middlewares.EtcdClientMiddleware(etcd)
	// Namespace - 命名空间中间件，处理多租户命名空间隔离
	namespaceMiddleware := middlewares.NamespaceMiddleware(&cfg.Namespace)
	// PipelineInspector - 管道检查中间件，将管道检查器注入请求上下文
	pipelineInspectorMiddleware := middlewares.PipelineInspectorMiddleware(inspector)
	// PipelineClient - 管道投递客户端中间件，将 asynq 投递客户端注入请求上下文
	pipelineClientMiddleware := middlewares.PipelineClientMiddleware(pipelineClient)
	// TagRuleEngine - 标签规则引擎中间件，将标签引擎注入请求上下文
	tagRuleEngineMiddleware := middlewares.TagRuleEngineMiddleware(tagEngine)
	// FeatureRegistry - 全局功能开关注册表中间件
	featureRegistryMiddleware := middlewares.FeatureRegistryMiddleware(featureRegistry)
	// MessagePool - 消息渠道连接池中间件
	messagePoolMiddleware := middlewares.MessagePoolMiddleware(messagePool)
	// SQLiteVTabPools - SQLite vtable 池持有者中间件
	sqliteVTabPoolsMiddleware := middlewares.SQLiteVTabPoolsMiddleware(sqliteVTabPools)
	// Ai - AI 助手运行时中间件
	aiMiddleware := middlewares.AiMiddleware(aiRuntime)

	// ==================== 默认错误处理 ====================
	// 重写Gin框架的默认错误响应，提供统一的错误格式和国际化支持

	// NoRoute - 处理404路由未找到错误
	engine.NoRoute(response.ErrDefaultPageNotFound.AbortWithGinH)
	// NoMethod - 处理405方法不允许错误
	engine.NoMethod(response.ErrDefaultNoMethod.AbortWithGinH)

	// ==================== 全局API端点 ====================
	// 这些端点不经过分组中间件，可用于健康检查和服务发现

	// /ping - 健康检查端点，用于服务健康监测和负载均衡器检查
	engine.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// ==================== API路由注册 ====================
	// 以下部分注册所有业务API路由，按照功能模块进行分组

	// 创建API版本根路径组：/api/v1
	// 所有业务API都挂载在此路径下，便于版本管理和路由组织
	rc := &views.RegisterContext{
		Engine:            engine,
		V1Group:           engine.Group("/api/v1"),
		Injector:          i,
		Authenticator:     authenticatorMiddleware,
		ForceAuthenticate: forceAuthenticateMiddleware,
		Policy:            policyMiddleware,
		Authorize:         authorizeMiddleware,
		CoreDatabase:      coreDatabaseMiddleware,
		RawDatabase:       rawDatabaseMiddleware,
		S3:                s3Middleware,
		ObjectCache:       objectCacheMiddleware,
		Etcd:              etcdMiddleware,
		Namespace:         namespaceMiddleware,
		PipelineInspector: pipelineInspectorMiddleware,
		PipelineClient:    pipelineClientMiddleware,
		TagRuleEngine:     tagRuleEngineMiddleware,
		FeatureRegistry:   featureRegistryMiddleware,
		MessagePool:       messagePoolMiddleware,
		SQLiteVTabPools:   sqliteVTabPoolsMiddleware,
		Ai:                aiMiddleware,
	}

	// ==================== 全局功能开关端点 ====================
	// /features/list - 公共接口（无需认证）：返回全部功能开关（key + enabled），
	// 供前端获知某项功能是否开启以决定是否展示相关组件。
	rc.V1Group.POST(
		"/features/list",
		rc.FeatureRegistry, // 注入功能开关注册表，读取各功能启停状态
		views.FeaturesListGet,
	)
	// 功能开关列表元信息（仅输出，无请求体）
	rc.V1Group.OPTIONS("/features/list", views.StructMetaGinHandler(
		views.WithMethod(http.MethodPost, views.WithBodyOutput[views.FeaturesListGetOut]()),
	))

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
	return nil
}

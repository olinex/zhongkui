// Package router 提供HTTP路由注册功能。
//
// 本包负责：
//  1. 初始化全局与分组中间件链
//  2. 按 path 拆分注册各组 API 路由（admin / traffic / tag-event / plaintext / user / notification / etl），
//     路由处理函数与元信息定义于 internal/manager/views 包
//
// 路由分组结构：
//   - /api/v1                   API版本根路径
//     ├── /admin                管理接口（需要认证和授权）
//     ├── /traffic              原始流量审计接口
//     ├── /tag-event            标签事件接口
//     ├── /plaintext            通用明文查看接口
//     ├── /user                 用户接口
//     │   ├── 公共接口          注册、登录等无需认证的接口
//     │   └── /self             个人接口（需要用户认证）
//     ├── /notification         通知管理接口
//     └── /etl                  ETL管道监控接口
package router

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// RegisterContext 路由注册上下文：聚合全局引擎、API 版本组与公共中间件实例，
// 供各 path 分组注册函数复用。
type RegisterContext struct {
	engine  *gin.Engine
	v1Group *gin.RouterGroup

	authenticatorMiddleware     gin.HandlerFunc
	forceAuthenticateMiddleware gin.HandlerFunc
	policyMiddleware            gin.HandlerFunc
	coreDatabaseMiddleware      gin.HandlerFunc
	rawDatabaseMiddleware       gin.HandlerFunc
	s3Middleware                gin.HandlerFunc
	etcdMiddleware              gin.HandlerFunc
	namespaceMiddleware         gin.HandlerFunc
	pipelineInspectorMiddleware gin.HandlerFunc
	pipelineClientMiddleware    gin.HandlerFunc
	tagRuleEngineMiddleware     gin.HandlerFunc
}

// RouteRegister 注册全部 HTTP 路由：初始化全局中间件链后，
// 按 path 委托各分组注册函数完成路由注册。
func RouteRegister(ctx context.Context, i do.Injector) error {
	viper := provider.MustInvokeViper(i)
	kd := provider.MustInvokeKeyDerivation(i)
	etcd := provider.MustInvokeEtcdClient(i)
	coreDB := provider.MustInvokeCoreDatabase(i)
	rawDB := provider.MustInvokeRawDatabase(i)
	s3Client := provider.MustInvokeS3(i)

	// 初始化命名空间缓存：设置数据库引用并预加载全量命名空间
	core.Cache.SetDB(coreDB)
	if err := core.Cache.Load(ctx); err != nil {
		return errors.WithStack(err)
	}

	engine := provider.MustInvokeGin(i)
	inspector := provider.MustInvokePipelineInspector(i)
	pipelineClient := provider.MustInvokePipelineClient(i)
	apiLogger := provider.MustInvokeApiLogger(i)
	rbac := provider.MustInvokeRbac(i)
	tagEngine := provider.MustInvokeTagEngine(i)
	cfg, err := manager.ExtractConfig(viper, validator.Validate)
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
	engine.Use(manager.ConfigMiddleware(cfg))
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

	// authenticatorMiddleware - 认证器中间件，将认证器实例注入请求上下文
	authenticatorMiddleware := authenticator.AuthenticatorMiddleware()
	// forceAuthenticateMiddleware - 强制认证中间件，要求请求必须通过JWT认证
	forceAuthenticateMiddleware := authenticator.AuthenticateMiddleware(false)
	// policyMiddleware - RBAC策略中间件，将策略引擎注入请求上下文
	policyMiddleware := middlewares.RbacPolicyMiddleware(rbac)
	// coreDatabaseMiddleware - 数据库中间件，将核心数据库连接注入请求上下文
	coreDatabaseMiddleware := middlewares.CoreDatabaseMiddleware(coreDB)
	// rawDatabaseMiddleware - raw 库中间件，供标签事件规则 dry-run 查询 tag_summings
	rawDatabaseMiddleware := middlewares.RawDatabaseMiddleware(rawDB)
	// s3Middleware - S3 客户端中间件，供原始流量 body 明文读取
	s3Middleware := middlewares.S3ClientMiddleware(s3Client)
	// etcdMiddleware - etcd客户端中间件，将etcd客户端注入请求上下文
	etcdMiddleware := middlewares.EtcdClientMiddleware(etcd)
	// namespaceMiddleware - 命名空间中间件，处理多租户命名空间隔离
	namespaceMiddleware := middlewares.NamespaceMiddleware(&cfg.Namespace)
	// authorizeMiddleware := middlewares.RbacPolicyAuthorizeMiddleware() - 权限验证中间件（暂未启用）
	pipelineInspectorMiddleware := middlewares.PipelineInspectorMiddleware(inspector)
	// pipelineClientMiddleware - 管道投递客户端中间件，将 asynq 投递客户端注入请求上下文，供定时任务"立即执行"
	pipelineClientMiddleware := middlewares.PipelineClientMiddleware(pipelineClient)
	// tagRuleEngineMiddleware - 标签规则引擎中间件，将标签引擎注入请求上下文，
	tagRuleEngineMiddleware := middlewares.TagRuleEngineMiddleware(tagEngine)

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
	rc := &RegisterContext{
		engine:                      engine,
		v1Group:                     engine.Group("/api/v1"),
		authenticatorMiddleware:     authenticatorMiddleware,
		forceAuthenticateMiddleware: forceAuthenticateMiddleware,
		policyMiddleware:            policyMiddleware,
		coreDatabaseMiddleware:      coreDatabaseMiddleware,
		rawDatabaseMiddleware:       rawDatabaseMiddleware,
		s3Middleware:                s3Middleware,
		etcdMiddleware:              etcdMiddleware,
		namespaceMiddleware:         namespaceMiddleware,
		pipelineInspectorMiddleware: pipelineInspectorMiddleware,
		pipelineClientMiddleware:    pipelineClientMiddleware,
		tagRuleEngineMiddleware:     tagRuleEngineMiddleware,
	}

	registerAdminRoutes(rc)
	registerTrafficRoutes(rc)
	registerTagEventRoutes(rc)
	registerPlaintextRoutes(rc)
	registerUserRoutes(rc)
	registerNotificationRoutes(rc)
	registerEtlRoutes(rc)
	return nil
}

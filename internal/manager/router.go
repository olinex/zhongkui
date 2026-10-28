// Package manager 提供 API 管理层的核心能力：HTTP 服务的启动与生命周期编排、配置管理、JWT 认证
// 与 RBAC 授权中间件，以及把路由分发给各业务子包的 RegisterRoutes。
//
// ## 定位与分层
//
// 管理端（internal/manager）是系统对外的 HTTP API 边界：它把「请求 → 鉴权 → 业务查询/写入 →
// 统一响应」这条同步链路收敛在一处，与异步侧的 internal/worker、装配侧的 internal/provider
// 形成清晰分工——worker 负责队列消费与后台任务，provider 负责依赖装配，本包只关心如何把 HTTP
// 请求正确地路由、授权、执行与回写。包内经下列层次自内向外展开：
//
//   - router.go：路由装配薄层，负责中间件链与公共端点，并把各业务域委托给子包；
//   - views/*（子包）：各业务域的 HTTP 处理器与 In/Out 视图结构体，按域内聚；
//   - response：统一响应与错误码体系，是全站错误语义的单一出口；
//   - middlewares：鉴权、授权、多租户、可观测性与各类依赖注入中间件；
//   - features：全局功能开关注册表，供前端与视图层共用「功能是否可用」的判断；
//   - config：管理端配置的提取与校验；service.go：HTTP 服务生命周期编排。
//
// ## 路由装配
//
// RouteRegister 是装配入口，其顺序本身即隐含约束：先初始化命名空间缓存（core.Cache.SetDB 后
// Load，使多租户隔离在首个请求前就绪），再经 datasourcepkg.Reload 依据已启用数据源恢复 SQLite
// 虚拟表注册（保证重启后 rego sqlite.query 立即可查），随后从注入器取用各中间件所依赖的实例
// （Etcd / 核心库与 raw 库 / S3 / ObjectCache / Pipeline / TagEngine / FeatureRegistry /
// MessagePool / AI 运行时等），最后构造 views.RegisterContext 并逐个调用子包的 RegisterRoutes。
// RegisterContext 把全局引擎、/api/v1 版本组、注入器与全部中间件实例聚合为一个可复用的注册句柄，
// 使每个业务子包都能在自身目录内完成自己域内的路由声明，避免路由散落在包外，也让「新增一个业务域」
// 退化为新增一个 views 子包并在 RouteRegister 中挂一行。
//
// ## 中间件分层
//
// 中间件分两级，作用域不同。全局链对所有请求生效，顺序即执行顺序：gin.Recovery 兜底 panic，
// OtelMiddleware 记录追踪与指标，ConfigMiddleware 注入配置，SecureHeaders / CORS 设置安全与跨域
// 响应头，ZapLoggerMiddleware 输出结构化访问日志，TranslateMiddleware 注入多语言本地化器。分组
// 中间件按路由组选择性挂载：认证（ForceAuthenticate 强制 JWT）与 RBAC 授权（Policy 注入策略引擎，
// Authorize 依据请求路径 api:{FullPath} 与 HTTP 方法动态校验权限，权限点取值由 `rbac_policy_templates`
// 约束）构成访问控制主干；Core/Raw 数据库、S3/ObjectCache、Etcd、Namespace、Pipeline
// Inspector/Client、TagRuleEngine、FeatureRegistry、MessagePool、SQLiteVTabPools 与 Ai 等中间件
// 把请求所需的具体依赖注入上下文，使处理器无需自行获取依赖、只消费上下文中的实例。
//
// ## 统一错误与公共端点
//
// 引擎以 NoRoute / NoMethod 重写 Gin 默认 404/405，统一走 ErrDefaultPageNotFound 与
// ErrDefaultNoMethod，保证错误体格式一致。/ping 提供健康检查；/features/list 为无需认证的公共
// 端点，返回全部功能开关（键 + 启停，必要时附不可用原因），供前端据此把入口置为不可用态而非隐藏。
//
// ## 响应与错误码体系
//
// response 包以 CodeError 统一承载 HTTP 状态码、业务错误码与详情，经 ToGinH / ToGinW 序列化，
// 经 AbortWithGinH / AbortWithGinW 直接中断请求。错误码分两段：基础码 1000-1099（CodeUnknown、
// CodeBadBody、CodeBadQuery、CodeBadUrlParam、CodeBadJwtToken、CodeAuthorizeFailed 等），业务码按领域
// 区间分配（1100-1199 通用与账户 / 1200-1299 标签 / 1300-1399 AI 助手 / 1400-1499 平台与运维；如
// CodeLlmBindingIncompatible=1404 表示档位绑定与系统当前配置不兼容）。错误码同时
// 作为 i18n 消息键（`code.{码}`），详情可经 DetailContext 按请求语言本地化。
//
// ## 视图层与 In/Out 解耦
//
// views 层为每个接口定义独立的 In/Out 结构体与处理器，并以内置基础设施支撑：HeaderOption 与
// getJSONTagAndType 反射导出列表列元数据供前端动态渲染表格；StructMetaGinHandler 搭配 WithMethod /
// WithBodyOutput 为同一路径提供 OPTIONS 元信息；GetRawParam / GetUint64Param / GetUUIDParam 统一
// 解析路径参数并在非法时中断请求。
//
// 视图层受一条强约束：In/Out 结构体禁止直接引用模型层（internal/models 下 core / network / tag）的
// 结构体——既不得结构体嵌入，也不得作为字段类型；即便字段完全相同也必须独立定义。处理器先以模型
// 查询数据库，再经显式映射函数 newXxxOut（入参为模型指针）逐字段转换为独立 Out，且 json 标签须与
// 来源字段保持一致以免破坏前端契约。该约束把「外部 API 形状」与「内部持久化形状」解耦：模型演进
// 不会意外泄漏为接口契约变更，接口演进也不必迁就模型，二者各自独立演化。模型专属类型（如
// network.LayerLevel / TCPOptions）同样不得出现，一律以视图层等价类型替换；唯一例外是仅用于 GORM
// 扫描的局部匿名容器。
//
// ## 功能开关与前端协作
//
// features 注册表以 Feature 接口（Key + Enabled）聚合各功能开关，Enabled 必须是能真实反映可用性的
// 派生判断（如密码重置依赖邮件能力、AI 助手依赖档位与 embedding 模型绑定），并可经 Reasoned 接口
// 按请求语言补充「不可用原因」。视图层与前端共用同一注册表，使功能可用性判断收敛到单一事实来源，
// 避免各处理器直接读取底层配置各自判断。
//
// ## 服务生命周期
//
// service.go 的 RunHttpService 在 goroutine 中启动 HTTP 服务，并以 select 等待 context 取消或启动
// 错误：收到信号即调用 srv.Shutdown 优雅关闭，启动失败则记录后返回错误。
//
// 本文件（router.go）承载全局中间件装配、公共端点注册与路由分发；原 internal/manager/router 子目录
// 已删除，此处并入 package manager（各业务域路由见 internal/manager/views/*/register.go）。
package manager

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"

	datasourcepkg "zhongkuisec.com/zhongkui/internal/datasource"
	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
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
		return errors.WithStack(err)
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
	cacheRedis := provider.MustInvokeCacheRedis(i)
	cfg, err := config.ExtractConfig(viper, validator.Validate)
	if err != nil {
		return errors.WithStack(err)
	}

	// 受信代理网段：Gin 默认信任所有代理并采信 X-Forwarded-For，来源地址可被伪造，
	// 故按配置收紧——默认空列表即不信任任何代理，ClientIP 退化为 TCP 对端地址；
	// 反向代理部署时须在 api.trustedProxies 显式填入代理网段。配置值已按网段类型
	// 在校验阶段解析（非法网段启动即失败），此处仅转换回 Gin 需要的文本列表
	trustedProxies := make([]string, 0, len(cfg.TrustedProxies))
	for _, proxy := range cfg.TrustedProxies {
		trustedProxies = append(trustedProxies, proxy.String())
	}
	if err := engine.SetTrustedProxies(trustedProxies); err != nil {
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
	// PasswordResetLimiter - 忘记密码申请限流中间件：按请求来源地址限制申请频率，
	// 抬升账号枚举与撞库配对探测的成本
	passwordResetLimiterMiddleware := middlewares.PasswordResetLimiterMiddleware(
		redis.NewFixedWindowLimiter(cacheRedis, cfg.PasswordReset.RateLimit, cfg.PasswordReset.RateLimitWindow),
	)
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
		Engine:               engine,
		V1Group:              engine.Group("/api/v1"),
		Injector:             i,
		Authenticator:        authenticatorMiddleware,
		ForceAuthenticate:    forceAuthenticateMiddleware,
		Policy:               policyMiddleware,
		Authorize:            authorizeMiddleware,
		CoreDatabase:         coreDatabaseMiddleware,
		RawDatabase:          rawDatabaseMiddleware,
		S3:                   s3Middleware,
		ObjectCache:          objectCacheMiddleware,
		Etcd:                 etcdMiddleware,
		Namespace:            namespaceMiddleware,
		PipelineInspector:    pipelineInspectorMiddleware,
		PipelineClient:       pipelineClientMiddleware,
		TagRuleEngine:        tagRuleEngineMiddleware,
		FeatureRegistry:      featureRegistryMiddleware,
		PasswordResetLimiter: passwordResetLimiterMiddleware,
		MessagePool:          messagePoolMiddleware,
		SQLiteVTabPools:      sqliteVTabPoolsMiddleware,
		Ai:                   aiMiddleware,
	}

	// ==================== 全局功能开关端点与业务路由 ====================
	// 全局功能开关端点（/features/list，公共接口）与全部视图域路由统一由 registerViewRoutes
	// 注册，使在线装配与离线接口目录装配（CollectApiCatalog）共用同一份域注册清单。
	registerViewRoutes(rc)
	// 能力声明随路由注册写入接口目录，此处把同一份快照注入 AI 运行时，仅供 api_invoke
	// 白名单校验；能力发现走 knowledge_search 的 capability 分区，不常驻模型上下文。
	aiRuntime.SetCapabilities(views.CapabilityItems())

	return nil
}

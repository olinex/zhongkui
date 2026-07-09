// Package routes 提供HTTP路由注册和管理功能。
//
// 本包负责：
// 1. 定义所有API路由及其对应的处理函数
// 2. 配置全局和分组的中间件链
// 3. 组织路由结构，包括公共路由、用户路由、管理路由等
//
// 路由分组结构：
//   - /api/v1                   API版本根路径
//     ├── /admin                管理接口（需要认证和授权）
//     ├── /user                 用户接口
//     │   ├── 公共接口          注册、登录等无需认证的接口
//     │   └── /self             个人接口（需要用户认证）
//     └── /etl                  ETL管道监控接口
package routes

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/provider"
)

func RouteRegister(ctx context.Context, i do.Injector) error {
	viper := provider.MustInvokeViper(i)
	kd := provider.MustInvokeKeyDerivation(i)
	etcd := provider.MustInvokeEtcdClient(i)
	coreDB := provider.MustInvokeCoreDatabase(i)

	// 初始化命名空间缓存：设置数据库引用并预加载全量命名空间
	core.Cache.SetDB(coreDB)
	if err := core.Cache.Load(ctx); err != nil {
		return errors.WithStack(err)
	}

	engine := provider.MustInvokeGin(i)
	inspector := provider.MustInvokePipelineInspector(i)
	apiLogger := provider.MustInvokeApiLogger(i)
	i18n := provider.MustInvokeI18n(i)
	rbac := provider.MustInvokeRbac(i)
	cfg, err := manager.ExtractConfig(viper)
	if err != nil {
		return errors.WithStack(err)
	}

	// 创建JWT认证器，用于用户身份验证和令牌管理
	authenticator := manager.NewJwtAuthenticator(&cfg.Auth, coreDB, kd)

	// ==================== 全局中间件配置 ====================
	// 以下中间件将应用于所有HTTP请求，按照添加顺序执行

	// gin.Recovery() - Gin框架的崩溃恢复中间件，防止未处理的panic导致服务崩溃
	engine.Use(gin.Recovery())
	// OtelMiddleware - OTEL中间件，用于记录请求跟踪和指标
	engine.Use(manager.OtelMiddleware())
	// ConfigMiddleware - 配置中间件，将应用程序配置注入到请求上下文
	engine.Use(manager.ConfigMiddleware(cfg))
	// SecureHeadersMiddleware - 安全响应头中间件，设置CSP、HSTS等安全头
	engine.Use(manager.SecureHeadersMiddleware(&cfg.Secure))
	// CORSHeadersMiddleware - CORS跨域中间件，处理跨域请求
	engine.Use(manager.CORSHeadersMiddleware(&cfg.Secure.CORS))
	// ZapLoggerMiddleware - 结构化日志中间件，记录请求详情和性能指标
	engine.Use(manager.ZapLoggerMiddleware(apiLogger))
	// TranslateMiddleware - 国际化中间件，支持多语言错误消息和界面
	engine.Use(manager.TranslateMiddleware(i18n))

	// ==================== 分组中间件配置 ====================
	// 以下中间件将根据路由分组选择性应用，提供细粒度的访问控制和功能支持

	// authenticatorMiddleware - 认证器中间件，将认证器实例注入请求上下文
	authenticatorMiddleware := authenticator.AuthenticatorMiddleware()
	// forceAuthenticateMiddleware - 强制认证中间件，要求请求必须通过JWT认证
	forceAuthenticateMiddleware := authenticator.AuthenticateMiddleware(false)
	// policyMiddleware - RBAC策略中间件，将策略引擎注入请求上下文
	policyMiddleware := manager.RbacPolicyMiddleware(rbac)
	// dbMiddleware - 数据库中间件，将核心数据库连接注入请求上下文
	dbMiddleware := manager.DatabaseMiddleware(coreDB)
	// etcdMiddleware - etcd客户端中间件，将etcd客户端注入请求上下文
	etcdMiddleware := manager.EtcdClientMiddleware(etcd)
	// namespaceMiddleware - 命名空间中间件，处理多租户命名空间隔离
	namespaceMiddleware := manager.NamespaceMiddleware(&cfg.Namespace)
	// authorizeMiddleware := manager.RbacPolicyAuthorizeMiddleware() - 权限验证中间件（暂未启用）
	pipelineInspectorMiddleware := manager.PipelineInspectorMiddleware(inspector)

	// ==================== 默认错误处理 ====================
	// 重写Gin框架的默认错误响应，提供统一的错误格式和国际化支持

	// NoRoute - 处理404路由未找到错误
	engine.NoRoute(manager.ErrDefaultPageNotFound.AbortWithGinH)
	// NoMethod - 处理405方法不允许错误
	engine.NoMethod(manager.ErrDefaultNoMethod.AbortWithGinH)

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
	v1Group := engine.Group("/api/v1")

	// ==================== 管理接口组：/api/v1/admin ====================
	// 管理接口需要完整的认证和授权，用于系统管理和配置

	adminGroup := v1Group.Group(
		"/admin",                    // 管理接口路径前缀
		authenticatorMiddleware,     // 认证器注入
		forceAuthenticateMiddleware, // 强制JWT认证
		policyMiddleware,            // RBAC策略引擎
		dbMiddleware,                // 数据库连接
		namespaceMiddleware,         // 命名空间隔离
		// authorizeMiddleware,       // 权限验证（暂未启用）
	)
	{
		// ========== 用户管理接口 ==========
		// 提供用户信息的增删改查和状态管理功能

		// 用户列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/user/list", userListGet)
		// 用户列表选项查询 - 返回可用的搜索字段和过滤条件
		adminGroup.OPTIONS("/user/list", ListOptionsGinHandler[*UserListGetSearchIn, *UserListOut])
		// 用户详情查询 - 根据unionID获取用户详细信息
		adminGroup.POST("/user/:unionID/get", userDetailGet)
		// 用户信息更新 - 更新用户基本信息
		adminGroup.POST("/user/:unionID/update", userUpdate)
		// 用户激活 - 启用用户账户
		adminGroup.POST("/user/:unionID/activate", userSetActive(true))
		// 用户停用 - 禁用用户账户
		adminGroup.POST("/user/:unionID/deactivate", userSetActive(false))
		// 用户加入用户组 - 将用户添加到指定用户组
		adminGroup.POST("/user/:unionID/join-group", userGroupJoin)
		// 用户离开用户组 - 将用户从指定用户组移除
		adminGroup.POST("/user/:unionID/leave-group", userGroupLeave)
		// ========== 用户组管理接口 ==========
		// 管理用户组及其RBAC权限策略

		// 用户组列表查询 - 获取所有用户组信息
		adminGroup.POST("/group/list", groupListGet)
		// 用户组列表选项查询 - 返回可用的搜索字段和过滤条件
		adminGroup.OPTIONS("/group/list", ListOptionsGinHandler[*GroupListGetSearchIn, *GroupListOut])
		// 创建用户组 - 创建新的用户组
		adminGroup.POST("/group/create", groupCreate)
		// 用户组详情查询 - 根据名称获取用户组详细信息
		adminGroup.POST("/group/:name/get", groupDetailGet)
		// 用户组信息更新 - 更新用户组基本信息
		adminGroup.POST("/group/:name/update", groupUpdate)
		// 删除用户组 - 删除指定的用户组
		adminGroup.POST("/group/:name/delete", groupDelete)
		// 添加RBAC策略到用户组 - 为用户组分配权限策略
		adminGroup.POST("/group/:name/add-rbac-policy", groupRbacPolicyAdd)
		// 从用户组移除RBAC策略 - 移除用户组的权限策略
		adminGroup.POST("/group/:name/remove-rbac-policy", groupRbacPolicyRemove)
		// ========== RBAC策略模板管理接口 ==========
		// 管理预定义的RBAC权限策略模板

		// RBAC策略模板列表查询 - 获取所有可用的权限策略模板
		adminGroup.POST("/rbac-policy-template/list", rbacPolicyTemplateListGet)
		// RBAC策略模板列表选项查询 - 返回可用的搜索字段和过滤条件
		adminGroup.OPTIONS("/rbac-policy-template/list", ListOptionsGinHandler[*RbacPolicyTmplateListGetSearchIn, *RbacPolicyTemplateListOut])

		// ========== 命名空间管理接口 ==========
		// 管理多租户命名空间，实现资源隔离

		// 命名空间列表查询 - 获取所有命名空间信息
		adminGroup.POST("/namespace/list", namespaceListGet)
		// 命名空间列表选项查询 - 返回可用的搜索字段和过滤条件
		adminGroup.OPTIONS("/namespace/list", ListOptionsGinHandler[*NamespaceListGetSearchIn, *NamespaceListOut])
		// 创建命名空间 - 创建新的命名空间
		adminGroup.POST("/namespace/create", namespaceCreate)
		// 命名空间详情查询 - 根据名称获取命名空间详细信息
		adminGroup.POST("/namespace/:name/get", namespaceDetailGet)
		// 命名空间信息更新 - 更新命名空间配置
		adminGroup.POST("/namespace/:name/update", namespaceUpdate)
		// 删除命名空间 - 删除指定的命名空间
		adminGroup.POST("/namespace/:name/delete", namespaceDelete)

		// ========== 抓包器管理接口 ==========
		// 管理网络数据包抓取器配置，需要etcd中间件进行配置同步

		// 抓包器列表查询 - 获取所有抓包器配置
		adminGroup.POST("/capturer/list", capturerListGet)
		// 抓包器列表选项查询 - 返回可用的搜索字段和过滤条件
		adminGroup.OPTIONS("/capturer/list", ListOptionsGinHandler[*CapturerListGetSearchIn, *CapturerListOut])
		// 创建抓包器 - 创建新的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/create", etcdMiddleware, capturerCreate)
		// 抓包器详情查询 - 根据ID获取抓包器详细信息（需要etcd中间件）
		adminGroup.POST("/capturer/:id/get", etcdMiddleware, capturerDetailGet)
		// 抓包器信息更新 - 更新抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/update", etcdMiddleware, capturerUpdate)
		// 删除抓包器 - 删除指定的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/delete", etcdMiddleware, capturerDelete)
	}

	// ==================== 用户接口组：/api/v1/user ====================
	// 用户相关接口，包括公共接口和个人接口

	// 创建用户接口组，包含公共接口和个人接口
	userGroup := v1Group.Group("/user")
	{
		// ========== 公共接口（无需认证） ==========
		// 这些接口对所有用户开放，用于账户注册和登录

		// 用户注册 - 创建新用户账户（需要数据库中间件）
		userGroup.POST(
			"/register",
			dbMiddleware, // 数据库连接，用于创建用户记录
			userRegister, // 用户注册处理函数
		)
		// 密码登录 - 使用用户名密码进行登录认证
		userGroup.POST(
			"/password/login",
			authenticatorMiddleware, // 认证器注入，用于生成JWT令牌
			dbMiddleware,            // 数据库连接，用于验证用户凭证
			userPasswordLogin,       // 密码登录处理函数
		)
	}
	{
		// ========== 个人接口（需要认证） ==========
		// 这些接口需要用户已登录认证，用于管理个人账户和资源

		// 创建个人接口子组：/api/v1/user/self
		selfGroup := userGroup.Group("/self")

		// 密码重置 - 用户修改自己的登录密码
		selfGroup.POST(
			"/password/reset",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证，确保用户已登录
			dbMiddleware,                // 数据库连接，用于更新密码
			userSelfPasswordReset,       // 密码重置处理函数
		)
		// JWT令牌刷新 - 刷新过期的JWT访问令牌
		selfGroup.POST(
			"/jwt/refresh",
			authenticatorMiddleware, // 认证器注入，用于生成新令牌
			userSelfJwtRefresh,      // JWT刷新处理函数
		)
		// 授权信息获取 - 获取用户的RBAC权限和可访问的命名空间
		selfGroup.POST(
			"/authorization-info/get",
			authenticatorMiddleware,      // 认证器注入
			forceAuthenticateMiddleware,  // 强制认证
			dbMiddleware,                 // 数据库连接，查询权限信息
			userSelfAuthorizationInfoGet, // 授权信息获取处理函数
		)
		// 个人资料修改 - 更新用户的昵称、邮箱等个人信息
		selfGroup.POST(
			"/profile/change",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			dbMiddleware,                // 数据库连接，更新用户信息
			userSelfProfileChange,       // 个人资料修改处理函数
		)
		// 访问密钥列表获取 - 获取用户的所有访问密钥
		selfGroup.POST(
			"/access-key/list",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			dbMiddleware,                // 数据库连接，查询访问密钥
			userSelfAccessKeyListGet,    // 访问密钥列表获取处理函数
		)
		// 创建访问密钥 - 为用户创建新的访问密钥
		selfGroup.POST(
			"/access-key/create",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			dbMiddleware,                // 数据库连接，创建访问密钥
			userSelfAccessKeyCreate,     // 访问密钥创建处理函数
		)
		// 删除访问密钥 - 删除指定的访问密钥
		selfGroup.POST(
			"/access-key/:id/remove",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			dbMiddleware,                // 数据库连接，删除访问密钥
			userSelfAccessKeyRemove,     // 访问密钥删除处理函数
		)
		// 激活访问密钥 - 启用已禁用的访问密钥
		selfGroup.POST(
			"/access-key/:id/activate",
			authenticatorMiddleware,        // 认证器注入
			forceAuthenticateMiddleware,    // 强制认证
			dbMiddleware,                   // 数据库连接，更新密钥状态
			userSelfAccessKeyEnabled(true), // 访问密钥启用处理函数
		)
		// 停用访问密钥 - 禁用正在使用的访问密钥
		selfGroup.POST(
			"/access-key/:id/deactivate",
			authenticatorMiddleware,         // 认证器注入
			forceAuthenticateMiddleware,     // 强制认证
			dbMiddleware,                    // 数据库连接，更新密钥状态
			userSelfAccessKeyEnabled(false), // 访问密钥停用处理函数
		)
	}
	// ==================== ETL管道监控接口组：/api/v1/etl ====================
	// 提供ETL（Extract-Transform-Load）任务管道的监控和管理功能

	// 创建ETL接口组
	etlGroup := v1Group.Group("/etl")
	{
		// ========== 检查器子组：/api/v1/etl/inspector ==========
		// 提供异步任务队列的监控和管理，需要管道检查器中间件

		// 创建检查器子组，添加管道检查器中间件
		inspectorGroup := etlGroup.Group("/inspector", pipelineInspectorMiddleware)

		// 服务器列表查询 - 获取所有任务队列服务器信息
		inspectorGroup.POST("/server/list", etlPipelineInspectorServerListGet)
		// 调度器列表查询 - 获取所有任务调度器信息
		inspectorGroup.POST("/scheduler/list", etlPipelineInspectorSchedulerListGet)
		// 队列列表查询 - 获取所有任务队列信息
		inspectorGroup.POST("/queue/list", etlPipelineInspectorQueueListGet)
		// 队列恢复运行 - 恢复指定队列的任务处理
		inspectorGroup.POST("/queue/:name/unpause", etlPipelineInspectorQueueUnpause)
		// 队列暂停 - 暂停指定队列的任务处理
		inspectorGroup.POST("/queue/:name/pause", etlPipelineInspectorQueuePause)
		// 队列每日统计查询 - 获取队列的每日任务统计信息
		inspectorGroup.POST("/queue/daily-stat/list", etlPipelineInspectorDailyStatListGet)
		// 任务列表查询 - 获取所有任务信息
		inspectorGroup.POST("/task/list", etlPipelineInspectorTaskListGet)
		// 按状态删除任务 - 删除指定状态的所有任务
		inspectorGroup.POST("/task/state/delete", etlPipelineInspectorStateTaskDelete)
		// 按状态归档任务 - 归档指定状态的所有任务
		inspectorGroup.POST("/task/state/archive", etlPipelineInspectorStateTaskArchive)
		// 按状态运行任务 - 运行指定状态的所有任务
		inspectorGroup.POST("/task/state/run", etlPipelineInspectorStateTaskRun)
		// 删除单个任务 - 删除指定ID的任务
		inspectorGroup.POST("/task/:id/delete", etlPipelineInspectorTaskDelete)
		// 归档单个任务 - 归档指定ID的任务
		inspectorGroup.POST("/task/:id/archive", etlPipelineInspectorTaskArchive)
		// 运行单个任务 - 运行指定ID的任务
		inspectorGroup.POST("/task/:id/run", etlPipelineInspectorTaskRun)
		// 取消处理中的任务 - 取消正在处理的任务
		inspectorGroup.POST("/task/:id/cancel-processing", etlPipelineInspectorTaskCancelPorcessing)
	}
	return nil
}

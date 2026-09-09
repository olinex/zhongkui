package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerUserRoutes 注册 /api/v1/user 用户接口路由。
// 用户相关接口，包括公共接口和个人接口。
func registerUserRoutes(ctx *RegisterContext) {
	// 创建用户接口组，包含公共接口和个人接口
	userGroup := ctx.v1Group.Group("/user")
	{
		// ========== 公共接口（无需认证） ==========
		// 这些接口对所有用户开放，用于账户登录

		// 密码登录 - 使用用户名密码进行登录认证
		userGroup.POST(
			"/password/login",
			ctx.authenticatorMiddleware, // 认证器注入，用于生成JWT令牌
			ctx.coreDatabaseMiddleware,  // 数据库连接，用于验证用户凭证
			views.UserPasswordLogin,     // 密码登录处理函数
		)
		// 密码登录元信息
		userGroup.OPTIONS("/password/login", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserPasswordLoginIn](), views.WithBodyOutput[views.UserTokenOut]()),
		))
		// 忘记密码 - 未登录态提交账号名与邮箱，校验一致后发送带重置链接的邮件
		userGroup.POST(
			"/password/forgot",
			ctx.coreDatabaseMiddleware,    // 数据库连接，校验账号名/邮箱并落重置令牌
			ctx.featureRegistryMiddleware, // 功能开关注册表，判断密码重置功能是否可用
			ctx.pipelineClientMiddleware,  // 管道投递客户端，入队 mail:send 异步任务
			views.UserPasswordForgot,      // 忘记密码处理函数
		)
		// 忘记密码元信息
		userGroup.OPTIONS("/password/forgot", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserPasswordForgotIn]()),
		))
		// 重置密码 - 通过邮件链接携带的令牌设置新密码
		userGroup.POST(
			"/password/reset",
			ctx.coreDatabaseMiddleware,     // 数据库连接，校验令牌并更新密码
			views.UserPasswordResetByToken, // 重置密码处理函数
		)
		// 重置密码元信息
		userGroup.OPTIONS("/password/reset", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserPasswordResetByTokenIn]()),
		))

		// JWT令牌刷新 - 刷新过期的JWT访问令牌（豁免强制认证与授权：
		// 刷新窗口内用过期 access token 换发，强制认证会拒绝，且授权依赖的 GetUser 不可达）
		// 注册在 /user 组下的全局路径 /api/v1/user/jwt/refresh，与命名空间隔离的 /self 子组无关
		userGroup.POST(
			"/jwt/refresh",
			ctx.authenticatorMiddleware, // 认证器注入，用于生成新令牌
			views.UserSelfJwtRefresh,    // JWT刷新处理函数
		)
		// JWT令牌刷新元信息
		userGroup.OPTIONS("/jwt/refresh", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserTokenOut]())))
	}

	{
		// ========== 个人接口（需要认证） ==========
		// 这些接口需要用户已登录认证，用于管理个人账户和资源

		// 创建个人接口子组：/api/v1/user/self
		// 统一挂载认证 + RBAC 授权链：authenticator → forceAuthenticate → policy → namespace → authorize → coreDatabase
		selfGroup := userGroup.Group(
			"/self",
			ctx.authenticatorMiddleware,     // 认证器注入
			ctx.forceAuthenticateMiddleware, // 强制认证，确保用户已登录
			ctx.policyMiddleware,            // RBAC策略引擎
			ctx.namespaceMiddleware,         // 命名空间隔离
			ctx.authorizeMiddleware,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
			ctx.coreDatabaseMiddleware,      // 数据库连接
		)

		// 密码重置 - 用户修改自己的登录密码
		selfGroup.POST("/password/reset", views.UserSelfPasswordReset)
		// 密码重置元信息
		selfGroup.OPTIONS("/password/reset", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserPasswordResetIn]())))
		// 授权信息获取 - 获取用户的RBAC权限和可访问的命名空间
		selfGroup.POST("/authorization-info/get", views.UserSelfAuthorizationInfoGet)
		// 授权信息获取元信息
		selfGroup.OPTIONS("/authorization-info/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.AuthorizationInfoOut]())))
		// 个人资料修改 - 更新用户的昵称、邮箱等个人信息
		selfGroup.POST("/profile/change", views.UserSelfProfileChange)
		// 个人资料修改元信息
		selfGroup.OPTIONS("/profile/change", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserProfileChangeIn]())))
		// 访问密钥列表获取 - 获取用户的所有访问密钥
		selfGroup.POST("/access-key/list", views.UserSelfAccessKeyListGet)
		// 访问密钥列表元信息
		selfGroup.OPTIONS("/access-key/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserAccessKeyOut]())))
		// 创建访问密钥 - 为用户创建新的访问密钥
		selfGroup.POST("/access-key/create", views.UserSelfAccessKeyCreate)
		// 创建访问密钥元信息
		selfGroup.OPTIONS("/access-key/create", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserAccessKeyCreateIn](), views.WithBodyOutput[views.UserAccessKeyOut]()),
		))
		// 删除访问密钥 - 删除指定的访问密钥
		selfGroup.POST("/access-key/:id/remove", views.UserSelfAccessKeyRemove)
		// 删除访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/remove", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 激活访问密钥 - 启用已禁用的访问密钥
		selfGroup.POST("/access-key/:id/activate", views.UserSelfAccessKeyEnabled(true))
		// 激活访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/activate", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserAccessKeyOut]())))
		// 停用访问密钥 - 禁用正在使用的访问密钥
		selfGroup.POST("/access-key/:id/deactivate", views.UserSelfAccessKeyEnabled(false))
		// 停用访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/deactivate", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserAccessKeyOut]())))
		// 前端配置查询 - 获取当前用户保存的前端页面配置
		selfGroup.POST("/frontend-config/get", views.UserSelfFrontendConfigGet)
		// 前端配置查询元信息
		selfGroup.OPTIONS("/frontend-config/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserFrontendConfigOut]()),
		))
		// 前端配置同步 - 上传当前用户的前端页面配置（乐观锁并发校验）
		selfGroup.POST("/frontend-config/sync", views.UserSelfFrontendConfigSync)
		// 前端配置同步元信息
		selfGroup.OPTIONS("/frontend-config/sync", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserFrontendConfigSyncIn](), views.WithBodyOutput[views.UserFrontendConfigOut]()),
		))
		// 前端配置同步开关 - 设置当前用户是否开启前端配置同步
		selfGroup.POST("/frontend-config/sync-enabled/set", views.UserSelfFrontendConfigSyncEnabledSet)
		// 前端配置同步开关元信息
		selfGroup.OPTIONS("/frontend-config/sync-enabled/set", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserFrontendConfigSyncEnabledIn]()),
		))
	}
}

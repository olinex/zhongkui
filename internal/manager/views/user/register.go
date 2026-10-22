// Package user 提供用户认证域（注册 / 登录 / 令牌 / 资料）的 API 处理器与视图 In/Out 结构体。
package user

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
)

// RegisterRoutes 注册 /api/v1/user 用户接口路由。
// 用户相关接口，包括公共接口和个人接口。
func RegisterRoutes(ctx *views.RegisterContext) {
	// 创建用户接口组，包含公共接口和个人接口
	userGroup := ctx.V1Group.Group("/user")
	{
		// ========== 公共接口（无需认证） ==========
		// 这些接口对所有用户开放，用于账户登录

		// 密码登录 - 使用用户名密码进行登录认证
		userGroup.POST(
			"/password/login",
			ctx.Authenticator, // 认证器注入，用于生成JWT令牌
			ctx.CoreDatabase,  // 数据库连接，用于验证用户凭证
			UserPasswordLogin, // 密码登录处理函数
		)
		// 密码登录元信息
		views.RegisterApi(userGroup, "/password/login", "密码登录 - 使用用户名密码进行登录认证",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserPasswordLoginIn](), views.WithBodyOutput[UserTokenOut]()),
		)
		// 忘记密码 - 未登录态提交账号名与邮箱，校验一致后发送带重置链接的邮件
		userGroup.POST(
			"/password/forgot",
			ctx.CoreDatabase,         // 数据库连接，校验账号名/邮箱并落重置令牌
			ctx.FeatureRegistry,      // 功能开关注册表，判断密码重置功能是否可用
			ctx.PipelineClient,       // 管道投递客户端，入队 mail:send 异步任务
			ctx.PasswordResetLimiter, // 限流器，按请求来源地址维度限制申请频率，抬升枚举成本
			UserPasswordForgot,       // 忘记密码处理函数
		)
		// 忘记密码元信息
		views.RegisterApi(userGroup, "/password/forgot", "忘记密码 - 未登录态提交账号名与邮箱，校验一致后发送带重置链接的邮件",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserPasswordForgotIn]()),
		)
		// 重置密码 - 通过邮件链接携带的令牌设置新密码
		userGroup.POST(
			"/password/reset",
			ctx.CoreDatabase,         // 数据库连接，校验令牌并更新密码
			UserPasswordResetByToken, // 重置密码处理函数
		)
		// 重置密码元信息
		views.RegisterApi(userGroup, "/password/reset", "重置密码 - 通过邮件链接携带的令牌设置新密码",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserPasswordResetByTokenIn]()),
		)

		// JWT令牌刷新 - 刷新过期的JWT访问令牌（豁免强制认证与授权：
		// 刷新窗口内用过期 access token 换发，强制认证会拒绝，且授权依赖的 GetUser 不可达）
		// 注册在 /user 组下的全局路径 /api/v1/user/jwt/refresh，与命名空间隔离的 /self 子组无关
		userGroup.POST(
			"/jwt/refresh",
			ctx.Authenticator,  // 认证器注入，用于生成新令牌
			UserSelfJwtRefresh, // JWT刷新处理函数
		)
		// JWT令牌刷新元信息
		views.RegisterApi(userGroup, "/jwt/refresh", "JWT令牌刷新", views.WithMethod(http.MethodPost, views.WithBodyOutput[UserTokenOut]()))
	}

	{
		// ========== 个人接口（需要认证） ==========
		// 这些接口需要用户已登录认证，用于管理个人账户和资源

		// 创建个人接口子组：/api/v1/user/self
		// 统一挂载认证 + RBAC 授权链：authenticator → forceAuthenticate → policy → namespace → authorize → coreDatabase
		selfGroup := userGroup.Group(
			"/self",
			ctx.Authenticator,     // 认证器注入
			ctx.ForceAuthenticate, // 强制认证，确保用户已登录
			ctx.Policy,            // RBAC策略引擎
			ctx.Namespace,         // 命名空间隔离
			ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
			ctx.CoreDatabase,      // 数据库连接
		)

		// 密码重置 - 用户修改自己的登录密码
		selfGroup.POST("/password/reset", UserSelfPasswordReset)
		// 密码重置元信息
		views.RegisterApi(selfGroup, "/password/reset", "密码重置 - 用户修改自己的登录密码", views.WithMethod(http.MethodPost, views.WithBodyInput[UserPasswordResetIn]()))
		// 授权信息获取 - 获取用户的RBAC权限和可访问的命名空间
		selfGroup.POST("/authorization-info/get", UserSelfAuthorizationInfoGet)
		// 授权信息获取元信息
		views.RegisterApi(selfGroup, "/authorization-info/get", "授权信息获取 - 获取用户的RBAC权限和可访问的命名空间",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[AuthorizationInfoOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.permission",
				Domain:    "user",
				Intent:    "查看当前用户的 RBAC 权限与可访问命名空间",
				Entities:  []string{"rbac_policies", "namespaces"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 个人资料修改 - 更新用户的昵称、邮箱等个人信息
		selfGroup.POST("/profile/change", UserSelfProfileChange)
		// 个人资料修改元信息
		views.RegisterApi(selfGroup, "/profile/change", "个人资料修改 - 更新用户的昵称、邮箱等个人信息", views.WithMethod(http.MethodPost, views.WithBodyInput[UserProfileChangeIn]()))
		// 访问密钥列表获取 - 获取用户的所有访问密钥
		selfGroup.POST("/access-key/list", UserSelfAccessKeyListGet)
		// 访问密钥列表元信息
		views.RegisterApi(selfGroup, "/access-key/list", "访问密钥列表获取 - 获取用户的所有访问密钥",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[UserAccessKeyOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.access-key.list",
				Domain:    "user",
				Intent:    "查看当前用户的访问密钥",
				Entities:  []string{"access_keys"},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 创建访问密钥 - 为用户创建新的访问密钥
		selfGroup.POST("/access-key/create", UserSelfAccessKeyCreate)
		// 创建访问密钥元信息
		views.RegisterApi(selfGroup, "/access-key/create", "创建访问密钥 - 为用户创建新的访问密钥",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserAccessKeyCreateIn](), views.WithBodyOutput[UserAccessKeyOut]()),
		)
		// 删除访问密钥 - 删除指定的访问密钥
		selfGroup.POST("/access-key/:id/remove", UserSelfAccessKeyRemove)
		// 删除访问密钥元信息
		views.RegisterApi(selfGroup, "/access-key/:id/remove", "删除访问密钥 - 删除指定的访问密钥", views.WithMethod(http.MethodPost))
		// 激活访问密钥 - 启用已禁用的访问密钥
		selfGroup.POST("/access-key/:id/activate", UserSelfAccessKeyEnabled(true))
		// 激活访问密钥元信息
		views.RegisterApi(selfGroup, "/access-key/:id/activate", "激活访问密钥 - 启用已禁用的访问密钥", views.WithMethod(http.MethodPost, views.WithBodyOutput[UserAccessKeyOut]()))
		// 停用访问密钥 - 禁用正在使用的访问密钥
		selfGroup.POST("/access-key/:id/deactivate", UserSelfAccessKeyEnabled(false))
		// 停用访问密钥元信息
		views.RegisterApi(selfGroup, "/access-key/:id/deactivate", "停用访问密钥 - 禁用正在使用的访问密钥", views.WithMethod(http.MethodPost, views.WithBodyOutput[UserAccessKeyOut]()))
		// 前端配置查询 - 获取当前用户保存的前端页面配置
		selfGroup.POST("/frontend-config/get", UserSelfFrontendConfigGet)
		// 前端配置查询元信息
		views.RegisterApi(selfGroup, "/frontend-config/get", "前端配置查询 - 获取当前用户保存的前端页面配置",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[UserFrontendConfigOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.frontend-config.get",
				Domain:    "user",
				Intent:    "查看当前用户保存的前端页面配置",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
		// 前端配置同步 - 上传当前用户的前端页面配置（乐观锁并发校验）
		selfGroup.POST("/frontend-config/sync", UserSelfFrontendConfigSync)
		// 前端配置同步元信息
		views.RegisterApi(selfGroup, "/frontend-config/sync", "前端配置同步 - 上传当前用户的前端页面配置（乐观锁并发校验）",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserFrontendConfigSyncIn](), views.WithBodyOutput[UserFrontendConfigOut]()),
		)
		// 前端配置同步开关 - 设置当前用户是否开启前端配置同步
		selfGroup.POST("/frontend-config/sync-enabled/set", UserSelfFrontendConfigSyncEnabledSet)
		// 前端配置同步开关元信息
		views.RegisterApi(selfGroup, "/frontend-config/sync-enabled/set", "前端配置同步开关 - 设置当前用户是否开启前端配置同步",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserFrontendConfigSyncEnabledIn]()),
		)
	}
}

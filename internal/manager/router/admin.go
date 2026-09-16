package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerAdminRoutes 注册 /api/v1/admin 管理接口路由。
// 管理接口需要完整的认证和授权，用于系统管理和配置。
func registerAdminRoutes(ctx *RegisterContext) {
	adminGroup := ctx.v1Group.Group(
		"/admin",                        // 管理接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.namespaceMiddleware,         // 命名空间隔离
		ctx.authorizeMiddleware,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.coreDatabaseMiddleware,      // 数据库连接
	)
	{
		// ========== 服务状态接口 ==========
		// 读取 etcd 服务目录，向用户呈现各类服务的整体运行状态
		adminGroup.POST("/service/list", views.ServiceListGet(ctx.injector))
		adminGroup.OPTIONS("/service/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[views.ServiceListOut]()),
		))

		// ========== 用户管理接口 ==========
		// 提供用户信息的增删改查和状态管理功能

		// 用户列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/user/list", views.UserListGet)
		// 用户列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/user/list", views.ListMetaGinHandler[*views.UserListGetSearchIn, *views.UserListOut]())
		// 创建用户 - 管理员创建新用户账户
		adminGroup.POST("/user/create", views.UserCreate)
		// 创建用户元信息
		adminGroup.OPTIONS("/user/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserCreateIn]())))
		// 用户详情查询 - 根据unionID获取用户详细信息
		adminGroup.POST("/user/:unionID/get", views.UserDetailGet)
		// 用户详情元信息
		adminGroup.OPTIONS("/user/:unionID/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.UserDetailOut]())))
		// 用户信息更新 - 更新用户基本信息
		adminGroup.POST("/user/:unionID/update", views.UserUpdate)
		// 用户信息更新元信息
		adminGroup.OPTIONS("/user/:unionID/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserProfileChangeIn]())))
		// 用户激活 - 批量启用用户账户
		adminGroup.POST("/user/activate", views.UserBatchSetActive(true))
		// 用户激活元信息
		adminGroup.OPTIONS("/user/activate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserBatchActiveIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 用户停用 - 批量禁用用户账户
		adminGroup.POST("/user/deactivate", views.UserBatchSetActive(false))
		// 用户停用元信息
		adminGroup.OPTIONS("/user/deactivate", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserBatchActiveIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 管理员重置用户密码 - 强制重置指定用户的登录密码，可指定新密码或随机生成
		adminGroup.POST("/user/:unionID/password/reset", views.UserPasswordReset)
		// 管理员重置用户密码元信息
		adminGroup.OPTIONS("/user/:unionID/password/reset", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.AdminUserPasswordResetIn](), views.WithBodyOutput[views.AdminUserPasswordResetOut]()),
		))
		// 用户加入用户组 - 将用户添加到指定用户组
		adminGroup.POST("/user/:unionID/join-group", views.UserGroupJoin)
		// 用户加入用户组元信息
		adminGroup.OPTIONS("/user/:unionID/join-group", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserGroupIn](), views.WithBodyOutput[views.UserRbacPolicyDetailOut]()),
		))
		// 用户离开用户组 - 将用户从指定用户组移除
		adminGroup.POST("/user/:unionID/leave-group", views.UserGroupLeave)
		// 用户离开用户组元信息
		adminGroup.OPTIONS("/user/:unionID/leave-group", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.UserGroupIn]())))
		// 重置用户前端配置 - 清空指定用户保存的前端页面配置
		adminGroup.POST("/user/:unionID/frontend-config/reset", views.UserFrontendConfigReset)
		// 重置用户前端配置元信息
		adminGroup.OPTIONS("/user/:unionID/frontend-config/reset", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// ========== 用户组管理接口 ==========
		// 管理用户组及其RBAC权限策略

		// 用户组列表查询 - 获取所有用户组信息
		adminGroup.POST("/group/list", views.GroupListGet)
		// 用户组列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/group/list", views.ListMetaGinHandler[*views.GroupListGetSearchIn, *views.GroupListOut]())
		// 创建用户组 - 创建新的用户组
		adminGroup.POST("/group/create", views.GroupCreate)
		// 创建用户组元信息
		adminGroup.OPTIONS("/group/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.GroupCreateIn]())))
		// 用户组详情查询 - 根据名称获取用户组详细信息
		adminGroup.POST("/group/:name/get", views.GroupDetailGet)
		// 用户组详情元信息
		adminGroup.OPTIONS("/group/:name/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.GroupDetailOut]())))
		// 用户组信息更新 - 更新用户组基本信息
		adminGroup.POST("/group/:name/update", views.GroupUpdate)
		// 用户组信息更新元信息
		adminGroup.OPTIONS("/group/:name/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.GroupUpdateIn]())))
		// 删除用户组 - 批量删除指定的用户组
		adminGroup.POST("/group/delete", views.GroupBatchDelete)
		// 删除用户组元信息
		adminGroup.OPTIONS("/group/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.GroupBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 添加RBAC策略到用户组 - 为用户组分配权限策略
		adminGroup.POST("/group/:name/add-rbac-policy", views.GroupRbacPolicyAdd)
		// 添加RBAC策略到用户组元信息
		adminGroup.OPTIONS("/group/:name/add-rbac-policy", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.GroupRbacPolicyIn](), views.WithBodyOutput[views.GroupRbacPolicyDetailOut]()),
		))
		// 从用户组移除RBAC策略 - 移除用户组的权限策略
		adminGroup.POST("/group/:name/remove-rbac-policy", views.GroupRbacPolicyRemove)
		// 从用户组移除RBAC策略元信息
		adminGroup.OPTIONS("/group/:name/remove-rbac-policy", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.GroupRbacPolicyIn]())))
		// ========== RBAC策略模板管理接口 ==========
		// 管理预定义的RBAC权限策略模板

		// RBAC策略模板列表查询 - 获取所有可用的权限策略模板
		adminGroup.POST("/rbac-policy-template/list", views.RbacPolicyTemplateListGet)
		// RBAC策略模板列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/rbac-policy-template/list", views.ListMetaGinHandler[*views.RbacPolicyTmplateListGetSearchIn, *views.RbacPolicyTemplateListOut]())

		// ========== 命名空间管理接口 ==========
		// 管理多租户命名空间，实现资源隔离

		// 命名空间列表查询 - 获取所有命名空间信息
		adminGroup.POST("/namespace/list", views.NamespaceListGet)
		// 命名空间列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/namespace/list", views.ListMetaGinHandler[*views.NamespaceListGetSearchIn, *views.NamespaceListOut]())
		// 创建命名空间 - 创建新的命名空间
		adminGroup.POST("/namespace/create", views.NamespaceCreate)
		// 创建命名空间元信息
		adminGroup.OPTIONS("/namespace/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NamespaceCreateIn]())))
		// 命名空间详情查询 - 根据名称获取命名空间详细信息
		adminGroup.POST("/namespace/:name/get", views.NamespaceDetailGet)
		// 命名空间详情元信息
		adminGroup.OPTIONS("/namespace/:name/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.NamespaceDetailOut]())))
		// 命名空间信息更新 - 更新命名空间配置
		adminGroup.POST("/namespace/:name/update", views.NamespaceUpdate)
		// 命名空间信息更新元信息
		adminGroup.OPTIONS("/namespace/:name/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.NamespaceUpdateIn]())))
		// 删除命名空间 - 批量删除指定的命名空间
		adminGroup.POST("/namespace/delete", views.NamespaceBatchDelete)
		// 删除命名空间元信息
		adminGroup.OPTIONS("/namespace/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.NamespaceBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))

		// ========== 抓包器管理接口 ==========
		// 管理网络数据包抓取器配置，需要etcd中间件进行配置同步

		// 抓包器列表查询 - 获取所有抓包器配置
		adminGroup.POST("/capturer/list", views.CapturerListGet)
		// 抓包器列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/capturer/list", views.ListMetaGinHandler[*views.CapturerListGetSearchIn, *views.CapturerListOut]())
		// 创建抓包器 - 创建新的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/create", ctx.etcdMiddleware, views.CapturerCreate)
		// 创建抓包器元信息
		adminGroup.OPTIONS("/capturer/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.CapturerCreateIn]())))
		// 抓包器详情查询 - 根据ID获取抓包器详细信息（需要etcd中间件）
		adminGroup.POST("/capturer/:id/get", ctx.etcdMiddleware, views.CapturerDetailGet)
		// 抓包器详情元信息
		adminGroup.OPTIONS("/capturer/:id/get", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.CapturerDetailOut]())))
		// 抓包器信息更新 - 更新抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/update", ctx.etcdMiddleware, views.CapturerUpdate)
		// 抓包器信息更新元信息
		adminGroup.OPTIONS("/capturer/:id/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.CapturerConfigIn]())))
		// 删除抓包器 - 批量删除指定的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/delete", ctx.etcdMiddleware, views.CapturerBatchDelete)
		// 删除抓包器元信息
		adminGroup.OPTIONS("/capturer/delete", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.CapturerBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		))
		// 采集器运行状态列表 - 读取 etcd 中 reporter 代为写入的在线会话状态
		adminGroup.POST("/capturer/status/list", views.CapturerStatusListGet(ctx.injector))
		// 采集器运行状态列表元信息（仅输出，无请求体）
		adminGroup.OPTIONS("/capturer/status/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[views.CapturerStatusListOut]()),
		))

		// ========== 统一枚举获取接口 ==========
		// 前端获取枚举值（key/value）与可读 label 的唯一通道，值来源为 enum 注册表
		adminGroup.POST("/enum/get", views.EnumGet)
		// 枚举类查询元信息
		adminGroup.OPTIONS("/enum/get", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.EnumGetIn](), views.WithBodyOutput[views.EnumTypeOut]()),
		))
		// 枚举全量类查询 - 按语言返回全部枚举类条目
		adminGroup.POST("/enum/list", views.EnumList)
		// 枚举全量类查询元信息
		adminGroup.OPTIONS("/enum/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.EnumListIn](), views.WithBodyOutput[views.EnumListOut]()),
		))

		// ========== 依赖状态自省接口 ==========
		// 返回全部外部依赖（database / redis / etcd / s3 / mail / grpc / llm）的配置与连通状态
		adminGroup.POST("/deps/list", views.DependencyListGet(ctx.injector))
		// 依赖状态列表元信息（仅输出，无请求体）
		adminGroup.OPTIONS("/deps/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyOutput[views.DependencyListOut]()),
		))
	}
}

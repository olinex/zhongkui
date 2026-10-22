// RegisterRoutes 注册 /api/v1/admin 管理接口路由。
package admin

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/manager/views/user"
)

// RegisterRoutes 注册 /api/v1/admin 管理接口路由。
// 管理接口需要完整的认证和授权，用于系统管理和配置。
func RegisterRoutes(ctx *views.RegisterContext) {
	adminGroup := ctx.V1Group.Group(
		"/admin",              // 管理接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
		ctx.CoreDatabase,      // 数据库连接
	)
	{
		// ========== 服务状态接口 ==========
		// 读取 etcd 服务目录，向用户呈现各类服务的整体运行状态
		adminGroup.POST("/service/list", ServiceListGet(ctx.Injector))
		views.RegisterApi(adminGroup, "/service/list", "读取 etcd 服务目录，向用户呈现各类服务的整体运行状态",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[ServiceListOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.service.list",
				Domain:    "platform",
				Intent:    "查看平台服务运行状态",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)

		// ========== 用户管理接口 ==========
		// 提供用户信息的增删改查和状态管理功能

		// 用户列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/user/list", UserListGet)
		// 用户列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		views.RegisterListApi[*views.UserListGetSearchIn, *UserListOut](adminGroup, "/user/list", "用户列表查询 - 支持分页、搜索和排序",
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.list",
				Domain:    "user",
				Intent:    "查看平台用户列表",
				Entities:  []string{"users"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建用户 - 管理员创建新用户账户
		adminGroup.POST("/user/create", UserCreate)
		// 创建用户元信息
		views.RegisterApi(adminGroup, "/user/create", "创建用户 - 管理员创建新用户账户", views.WithMethod(http.MethodPost, views.WithBodyInput[UserCreateIn]()))
		// 用户详情查询 - 根据unionID获取用户详细信息
		adminGroup.POST("/user/:unionID/get", UserDetailGet)
		// 用户详情元信息
		views.RegisterApi(adminGroup, "/user/:unionID/get", "用户详情查询 - 根据unionID获取用户详细信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[UserDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.detail",
				Domain:    "user",
				Intent:    "查看用户详情",
				Entities:  []string{"users", "groups", "rbac_policies"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 用户信息更新 - 更新用户基本信息
		adminGroup.POST("/user/:unionID/update", UserUpdate)
		// 用户信息更新元信息
		views.RegisterApi(adminGroup, "/user/:unionID/update", "用户信息更新 - 更新用户基本信息", views.WithMethod(http.MethodPost, views.WithBodyInput[user.UserProfileChangeIn]()))
		// 用户激活 - 批量启用用户账户
		adminGroup.POST("/user/activate", UserBatchSetActive(true))
		// 用户激活元信息
		views.RegisterApi(adminGroup, "/user/activate", "用户激活 - 批量启用用户账户",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserBatchActiveIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 用户停用 - 批量禁用用户账户
		adminGroup.POST("/user/deactivate", UserBatchSetActive(false))
		// 用户停用元信息
		views.RegisterApi(adminGroup, "/user/deactivate", "用户停用 - 批量禁用用户账户",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserBatchActiveIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 管理员重置用户密码 - 强制重置指定用户的登录密码，可指定新密码或随机生成
		adminGroup.POST("/user/:unionID/password/reset", UserPasswordReset)
		// 管理员重置用户密码元信息
		views.RegisterApi(adminGroup, "/user/:unionID/password/reset", "管理员重置用户密码 - 强制重置指定用户的登录密码，可指定新密码或随机生成",
			views.WithMethod(http.MethodPost, views.WithBodyInput[AdminUserPasswordResetIn](), views.WithBodyOutput[AdminUserPasswordResetOut]()),
		)
		// 用户加入用户组 - 将用户添加到指定用户组
		adminGroup.POST("/user/:unionID/join-group", UserGroupJoin)
		// 用户加入用户组元信息
		views.RegisterApi(adminGroup, "/user/:unionID/join-group", "用户加入用户组 - 将用户添加到指定用户组",
			views.WithMethod(http.MethodPost, views.WithBodyInput[UserGroupIn](), views.WithBodyOutput[UserRbacPolicyDetailOut]()),
		)
		// 用户离开用户组 - 将用户从指定用户组移除
		adminGroup.POST("/user/:unionID/leave-group", UserGroupLeave)
		// 用户离开用户组元信息
		views.RegisterApi(adminGroup, "/user/:unionID/leave-group", "用户离开用户组 - 将用户从指定用户组移除", views.WithMethod(http.MethodPost, views.WithBodyInput[UserGroupIn]()))
		// 重置用户前端配置 - 清空指定用户保存的前端页面配置
		adminGroup.POST("/user/:unionID/frontend-config/reset", UserFrontendConfigReset)
		// 重置用户前端配置元信息
		views.RegisterApi(adminGroup, "/user/:unionID/frontend-config/reset", "重置用户前端配置 - 清空指定用户保存的前端页面配置", views.WithMethod(http.MethodPost))
		// ========== 用户组管理接口 ==========
		// 管理用户组及其RBAC权限策略

		// 用户组列表查询 - 获取所有用户组信息
		adminGroup.POST("/group/list", GroupListGet)
		// 用户组列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		views.RegisterListApi[*views.GroupListGetSearchIn, *GroupListOut](adminGroup, "/group/list", "用户组列表查询 - 获取所有用户组信息",
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.group.list",
				Domain:    "user",
				Intent:    "查看用户组列表",
				Entities:  []string{"groups"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建用户组 - 创建新的用户组
		adminGroup.POST("/group/create", GroupCreate)
		// 创建用户组元信息
		views.RegisterApi(adminGroup, "/group/create", "创建用户组 - 创建新的用户组", views.WithMethod(http.MethodPost, views.WithBodyInput[GroupCreateIn]()))
		// 用户组详情查询 - 根据名称获取用户组详细信息
		adminGroup.POST("/group/:name/get", GroupDetailGet)
		// 用户组详情元信息
		views.RegisterApi(adminGroup, "/group/:name/get", "用户组详情查询 - 根据名称获取用户组详细信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[GroupDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.group.detail",
				Domain:    "user",
				Intent:    "查看用户组详情及其 RBAC 策略",
				Entities:  []string{"groups", "rbac_policies"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 用户组信息更新 - 更新用户组基本信息
		adminGroup.POST("/group/:name/update", GroupUpdate)
		// 用户组信息更新元信息
		views.RegisterApi(adminGroup, "/group/:name/update", "用户组信息更新 - 更新用户组基本信息", views.WithMethod(http.MethodPost, views.WithBodyInput[GroupUpdateIn]()))
		// 删除用户组 - 批量删除指定的用户组
		adminGroup.POST("/group/delete", GroupBatchDelete)
		// 删除用户组元信息
		views.RegisterApi(adminGroup, "/group/delete", "删除用户组 - 批量删除指定的用户组",
			views.WithMethod(http.MethodPost, views.WithBodyInput[GroupBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 添加RBAC策略到用户组 - 为用户组分配权限策略
		adminGroup.POST("/group/:name/add-rbac-policy", GroupRbacPolicyAdd)
		// 添加RBAC策略到用户组元信息
		views.RegisterApi(adminGroup, "/group/:name/add-rbac-policy", "添加RBAC策略到用户组 - 为用户组分配权限策略",
			views.WithMethod(http.MethodPost, views.WithBodyInput[GroupRbacPolicyIn](), views.WithBodyOutput[GroupRbacPolicyDetailOut]()),
		)
		// 从用户组移除RBAC策略 - 移除用户组的权限策略
		adminGroup.POST("/group/:name/remove-rbac-policy", GroupRbacPolicyRemove)
		// 从用户组移除RBAC策略元信息
		views.RegisterApi(adminGroup, "/group/:name/remove-rbac-policy", "从用户组移除RBAC策略 - 移除用户组的权限策略", views.WithMethod(http.MethodPost, views.WithBodyInput[GroupRbacPolicyIn]()))
		// ========== RBAC策略模板管理接口 ==========
		// 管理预定义的RBAC权限策略模板

		// RBAC策略模板列表查询 - 获取所有可用的权限策略模板
		adminGroup.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)
		// RBAC策略模板列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		views.RegisterListApi[*views.RbacPolicyTemplateListGetSearchIn, *RbacPolicyTemplateListOut](adminGroup, "/rbac-policy-template/list", "RBAC策略模板列表查询 - 获取所有可用的权限策略模板",
			views.WithCapability(views.CapabilityMeta{
				ID:        "user.rbac-template.list",
				Domain:    "user",
				Intent:    "查看可用的 RBAC 策略模板",
				Entities:  []string{"rbac_policy_templates"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)

		// ========== 命名空间管理接口 ==========
		// 管理多租户命名空间，实现资源隔离

		// 命名空间列表查询 - 获取所有命名空间信息
		adminGroup.POST("/namespace/list", NamespaceListGet)
		// 命名空间列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		views.RegisterListApi[*views.NamespaceListGetSearchIn, *NamespaceListOut](adminGroup, "/namespace/list", "命名空间列表查询 - 获取所有命名空间信息",
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.namespace.list",
				Domain:    "platform",
				Intent:    "查看当前部署的命名空间",
				Entities:  []string{"namespaces"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建命名空间 - 创建新的命名空间
		adminGroup.POST("/namespace/create", NamespaceCreate)
		// 创建命名空间元信息
		views.RegisterApi(adminGroup, "/namespace/create", "创建命名空间 - 创建新的命名空间", views.WithMethod(http.MethodPost, views.WithBodyInput[NamespaceCreateIn]()))
		// 命名空间详情查询 - 根据名称获取命名空间详细信息
		adminGroup.POST("/namespace/:name/get", NamespaceDetailGet)
		// 命名空间详情元信息
		views.RegisterApi(adminGroup, "/namespace/:name/get", "命名空间详情查询 - 根据名称获取命名空间详细信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[NamespaceDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.namespace.detail",
				Domain:    "platform",
				Intent:    "查看命名空间详情",
				Entities:  []string{"namespaces"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 命名空间信息更新 - 更新命名空间配置
		adminGroup.POST("/namespace/:name/update", NamespaceUpdate)
		// 命名空间信息更新元信息
		views.RegisterApi(adminGroup, "/namespace/:name/update", "命名空间信息更新 - 更新命名空间配置", views.WithMethod(http.MethodPost, views.WithBodyInput[NamespaceUpdateIn]()))
		// 删除命名空间 - 批量删除指定的命名空间
		adminGroup.POST("/namespace/delete", NamespaceBatchDelete)
		// 删除命名空间元信息
		views.RegisterApi(adminGroup, "/namespace/delete", "删除命名空间 - 批量删除指定的命名空间",
			views.WithMethod(http.MethodPost, views.WithBodyInput[NamespaceBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)

		// ========== 抓包器管理接口 ==========
		// 管理网络数据包抓取器配置，需要etcd中间件进行配置同步

		// 抓包器列表查询 - 获取所有抓包器配置
		adminGroup.POST("/capturer/list", CapturerListGet)
		// 抓包器列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		views.RegisterListApi[*views.CapturerListGetSearchIn, *CapturerListOut](adminGroup, "/capturer/list", "抓包器列表查询 - 获取所有抓包器配置",
			views.WithCapability(views.CapabilityMeta{
				ID:        "capturer.list",
				Domain:    "capturer",
				Intent:    "查看采集器配置与运行状态",
				Entities:  []string{"capturers"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 创建抓包器 - 创建新的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/create", ctx.Etcd, CapturerCreate)
		// 创建抓包器元信息
		views.RegisterApi(adminGroup, "/capturer/create", "创建抓包器 - 创建新的抓包器配置（需要etcd中间件）", views.WithMethod(http.MethodPost, views.WithBodyInput[CapturerCreateIn]()))
		// 抓包器详情查询 - 根据ID获取抓包器详细信息（需要etcd中间件）
		adminGroup.POST("/capturer/:id/get", ctx.Etcd, CapturerDetailGet)
		// 抓包器详情元信息
		views.RegisterApi(adminGroup, "/capturer/:id/get", "抓包器详情查询 - 根据ID获取抓包器详细信息（需要etcd中间件）",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[CapturerDetailOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "capturer.detail",
				Domain:    "capturer",
				Intent:    "查看单个采集器的完整配置",
				Entities:  []string{"capturers"},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 抓包器信息更新 - 更新抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/update", ctx.Etcd, CapturerUpdate)
		// 抓包器信息更新元信息
		views.RegisterApi(adminGroup, "/capturer/:id/update", "抓包器信息更新 - 更新抓包器配置（需要etcd中间件）", views.WithMethod(http.MethodPost, views.WithBodyInput[CapturerConfigIn]()))
		// 删除抓包器 - 批量删除指定的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/delete", ctx.Etcd, CapturerBatchDelete)
		// 删除抓包器元信息
		views.RegisterApi(adminGroup, "/capturer/delete", "删除抓包器 - 批量删除指定的抓包器配置（需要etcd中间件）",
			views.WithMethod(http.MethodPost, views.WithBodyInput[CapturerBatchDeleteIn](), views.WithBodyOutput[views.BatchResultOut]()),
		)
		// 采集器运行状态列表 - 读取 etcd 中 reporter 代为写入的在线会话状态
		adminGroup.POST("/capturer/status/list", CapturerStatusListGet(ctx.Injector))
		// 采集器运行状态列表元信息（仅输出，无请求体）
		views.RegisterApi(adminGroup, "/capturer/status/list", "采集器运行状态列表 - 读取 etcd 中 reporter 代为写入的在线会话状态",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[CapturerStatusListOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "capturer.status.list",
				Domain:    "capturer",
				Intent:    "查看采集器在线运行状态",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)

		// ========== 统一枚举获取接口 ==========
		// 前端获取枚举值（key/value）与可读 label 的唯一通道，值来源为 enum 注册表
		adminGroup.POST("/enum/get", views.EnumGet)
		// 枚举类查询元信息
		views.RegisterApi(adminGroup, "/enum/get", "前端获取枚举值（key/value）与可读 label 的唯一通道，值来源为 enum 注册表",
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.EnumGetIn](), views.WithBodyOutput[views.EnumTypeOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.enum.get",
				Domain:    "platform",
				Intent:    "按枚举类与语言查询枚举条目",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 枚举全量类查询 - 按语言返回全部枚举类条目
		adminGroup.POST("/enum/list", views.EnumList)
		// 枚举全量类查询元信息
		views.RegisterApi(adminGroup, "/enum/list", "枚举全量类查询 - 按语言返回全部枚举类条目",
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.EnumListIn](), views.WithBodyOutput[views.EnumListOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.enum.list",
				Domain:    "platform",
				Intent:    "查询全部枚举类条目",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)

		// ========== 依赖状态自省接口 ==========
		// 返回全部外部依赖（database / redis / etcd / s3 / mail / grpc / llm）的配置与连通状态
		adminGroup.POST("/deps/list", DependencyListGet(ctx.Injector))
		// 依赖状态列表元信息（仅输出，无请求体）
		views.RegisterApi(adminGroup, "/deps/list", "返回全部外部依赖（database / redis / etcd / s3 / mail / grpc / llm）的配置与连通状态",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[DependencyListOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "platform.dependency.list",
				Domain:    "platform",
				Intent:    "查看外部依赖配置与连通状态",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: false,
			}),
		)
	}
}

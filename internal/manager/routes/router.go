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
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

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
	apiLogger := provider.MustInvokeApiLogger(i)
	rbac := provider.MustInvokeRbac(i)
	tagEngine := provider.MustInvokeTagEngine(i)
	cfg, err := manager.ExtractConfig(viper, validator.Validate)
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
	engine.Use(manager.TranslateMiddleware())

	// ==================== 分组中间件配置 ====================
	// 以下中间件将根据路由分组选择性应用，提供细粒度的访问控制和功能支持

	// authenticatorMiddleware - 认证器中间件，将认证器实例注入请求上下文
	authenticatorMiddleware := authenticator.AuthenticatorMiddleware()
	// forceAuthenticateMiddleware - 强制认证中间件，要求请求必须通过JWT认证
	forceAuthenticateMiddleware := authenticator.AuthenticateMiddleware(false)
	// policyMiddleware - RBAC策略中间件，将策略引擎注入请求上下文
	policyMiddleware := manager.RbacPolicyMiddleware(rbac)
	// coreDatabaseMiddleware - 数据库中间件，将核心数据库连接注入请求上下文
	coreDatabaseMiddleware := manager.CoreDatabaseMiddleware(coreDB)
	// rawDatabaseMiddleware - raw 库中间件，供标签事件规则 dry-run 查询 tag_summings
	rawDatabaseMiddleware := manager.RawDatabaseMiddleware(rawDB)
	// s3Middleware - S3 客户端中间件，供原始流量 body 明文读取
	s3Middleware := manager.S3ClientMiddleware(s3Client)
	// etcdMiddleware - etcd客户端中间件，将etcd客户端注入请求上下文
	etcdMiddleware := manager.EtcdClientMiddleware(etcd)
	// namespaceMiddleware - 命名空间中间件，处理多租户命名空间隔离
	namespaceMiddleware := manager.NamespaceMiddleware(&cfg.Namespace)
	// authorizeMiddleware := manager.RbacPolicyAuthorizeMiddleware() - 权限验证中间件（暂未启用）
	pipelineInspectorMiddleware := manager.PipelineInspectorMiddleware(inspector)
	// tagRuleEngineMiddleware - 标签规则引擎中间件，将标签引擎注入请求上下文，
	tagRuleEngineMiddleware := manager.TagRuleEngineMiddleware(tagEngine)

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
		coreDatabaseMiddleware,      // 数据库连接
		namespaceMiddleware,         // 命名空间隔离
		// authorizeMiddleware,       // 权限验证（暂未启用）
	)
	{
		// ========== 用户管理接口 ==========
		// 提供用户信息的增删改查和状态管理功能

		// 用户列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/user/list", userListGet)
		// 用户列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/user/list", ListMetaGinHandler[*UserListGetSearchIn, *UserListOut]())
		// 创建用户 - 管理员创建新用户账户
		adminGroup.POST("/user/create", userCreate)
		// 创建用户元信息
		adminGroup.OPTIONS("/user/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserCreateIn]())))
		// 用户详情查询 - 根据unionID获取用户详细信息
		adminGroup.POST("/user/:unionID/get", userDetailGet)
		// 用户详情元信息
		adminGroup.OPTIONS("/user/:unionID/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[UserDetailOut]())))
		// 用户信息更新 - 更新用户基本信息
		adminGroup.POST("/user/:unionID/update", userUpdate)
		// 用户信息更新元信息
		adminGroup.OPTIONS("/user/:unionID/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserProfileChangeIn]())))
		// 用户激活 - 启用用户账户
		adminGroup.POST("/user/:unionID/activate", userSetActive(true))
		// 用户激活元信息
		adminGroup.OPTIONS("/user/:unionID/activate", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 用户停用 - 禁用用户账户
		adminGroup.POST("/user/:unionID/deactivate", userSetActive(false))
		// 用户停用元信息
		adminGroup.OPTIONS("/user/:unionID/deactivate", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 管理员重置用户密码 - 强制重置指定用户的登录密码，可指定新密码或随机生成
		adminGroup.POST("/user/:unionID/password/reset", userPasswordReset)
		// 管理员重置用户密码元信息
		adminGroup.OPTIONS("/user/:unionID/password/reset", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[AdminUserPasswordResetIn](), WithBodyOutput[AdminUserPasswordResetOut]()),
		))
		// 用户加入用户组 - 将用户添加到指定用户组
		adminGroup.POST("/user/:unionID/join-group", userGroupJoin)
		// 用户加入用户组元信息
		adminGroup.OPTIONS("/user/:unionID/join-group", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[UserGroupIn](), WithBodyOutput[UserRbacPolicyDetailOut]()),
		))
		// 用户离开用户组 - 将用户从指定用户组移除
		adminGroup.POST("/user/:unionID/leave-group", userGroupLeave)
		// 用户离开用户组元信息
		adminGroup.OPTIONS("/user/:unionID/leave-group", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserGroupIn]())))
		// 重置用户前端配置 - 清空指定用户保存的前端页面配置
		adminGroup.POST("/user/:unionID/frontend-config/reset", userFrontendConfigReset)
		// 重置用户前端配置元信息
		adminGroup.OPTIONS("/user/:unionID/frontend-config/reset", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// ========== 用户组管理接口 ==========
		// 管理用户组及其RBAC权限策略

		// 用户组列表查询 - 获取所有用户组信息
		adminGroup.POST("/group/list", groupListGet)
		// 用户组列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/group/list", ListMetaGinHandler[*GroupListGetSearchIn, *GroupListOut]())
		// 创建用户组 - 创建新的用户组
		adminGroup.POST("/group/create", groupCreate)
		// 创建用户组元信息
		adminGroup.OPTIONS("/group/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[GroupCreateIn]())))
		// 用户组详情查询 - 根据名称获取用户组详细信息
		adminGroup.POST("/group/:name/get", groupDetailGet)
		// 用户组详情元信息
		adminGroup.OPTIONS("/group/:name/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[GroupDetailOut]())))
		// 用户组信息更新 - 更新用户组基本信息
		adminGroup.POST("/group/:name/update", groupUpdate)
		// 用户组信息更新元信息
		adminGroup.OPTIONS("/group/:name/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[GroupUpdateIn]())))
		// 删除用户组 - 删除指定的用户组
		adminGroup.POST("/group/:name/delete", groupDelete)
		// 删除用户组元信息
		adminGroup.OPTIONS("/group/:name/delete", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 添加RBAC策略到用户组 - 为用户组分配权限策略
		adminGroup.POST("/group/:name/add-rbac-policy", groupRbacPolicyAdd)
		// 添加RBAC策略到用户组元信息
		adminGroup.OPTIONS("/group/:name/add-rbac-policy", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[GroupRbacPolicyIn](), WithBodyOutput[GroupRbacPolicyDetailOut]()),
		))
		// 从用户组移除RBAC策略 - 移除用户组的权限策略
		adminGroup.POST("/group/:name/remove-rbac-policy", groupRbacPolicyRemove)
		// 从用户组移除RBAC策略元信息
		adminGroup.OPTIONS("/group/:name/remove-rbac-policy", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[GroupRbacPolicyIn]())))
		// ========== RBAC策略模板管理接口 ==========
		// 管理预定义的RBAC权限策略模板

		// RBAC策略模板列表查询 - 获取所有可用的权限策略模板
		adminGroup.POST("/rbac-policy-template/list", rbacPolicyTemplateListGet)
		// RBAC策略模板列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/rbac-policy-template/list", ListMetaGinHandler[*RbacPolicyTmplateListGetSearchIn, *RbacPolicyTemplateListOut]())

		// ========== 命名空间管理接口 ==========
		// 管理多租户命名空间，实现资源隔离

		// 命名空间列表查询 - 获取所有命名空间信息
		adminGroup.POST("/namespace/list", namespaceListGet)
		// 命名空间列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/namespace/list", ListMetaGinHandler[*NamespaceListGetSearchIn, *NamespaceListOut]())
		// 创建命名空间 - 创建新的命名空间
		adminGroup.POST("/namespace/create", namespaceCreate)
		// 创建命名空间元信息
		adminGroup.OPTIONS("/namespace/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[NamespaceCreateIn]())))
		// 命名空间详情查询 - 根据名称获取命名空间详细信息
		adminGroup.POST("/namespace/:name/get", namespaceDetailGet)
		// 命名空间详情元信息
		adminGroup.OPTIONS("/namespace/:name/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[NamespaceDetailOut]())))
		// 命名空间信息更新 - 更新命名空间配置
		adminGroup.POST("/namespace/:name/update", namespaceUpdate)
		// 命名空间信息更新元信息
		adminGroup.OPTIONS("/namespace/:name/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[NamespaceUpdateIn]())))
		// 删除命名空间 - 删除指定的命名空间
		adminGroup.POST("/namespace/:name/delete", namespaceDelete)
		// 删除命名空间元信息
		adminGroup.OPTIONS("/namespace/:name/delete", StructMetaGinHandler(WithMethod(http.MethodPost)))

		// ========== 抓包器管理接口 ==========
		// 管理网络数据包抓取器配置，需要etcd中间件进行配置同步

		// 抓包器列表查询 - 获取所有抓包器配置
		adminGroup.POST("/capturer/list", capturerListGet)
		// 抓包器列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/capturer/list", ListMetaGinHandler[*CapturerListGetSearchIn, *CapturerListOut]())
		// 创建抓包器 - 创建新的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/create", etcdMiddleware, capturerCreate)
		// 创建抓包器元信息
		adminGroup.OPTIONS("/capturer/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[CapturerCreateIn]())))
		// 抓包器详情查询 - 根据ID获取抓包器详细信息（需要etcd中间件）
		adminGroup.POST("/capturer/:id/get", etcdMiddleware, capturerDetailGet)
		// 抓包器详情元信息
		adminGroup.OPTIONS("/capturer/:id/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[CapturerDetailOut]())))
		// 抓包器信息更新 - 更新抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/update", etcdMiddleware, capturerUpdate)
		// 抓包器信息更新元信息
		adminGroup.OPTIONS("/capturer/:id/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[CapturerConfigIn]())))
		// 删除抓包器 - 删除指定的抓包器配置（需要etcd中间件）
		adminGroup.POST("/capturer/:id/delete", etcdMiddleware, capturerDelete)
		// 删除抓包器元信息
		adminGroup.OPTIONS("/capturer/:id/delete", StructMetaGinHandler(WithMethod(http.MethodPost)))

		// ========== 标签识别规则管理接口 ==========
		// 管理标签识别规则的增删改查、启停、校验、dry-run 与版本回滚

		// 标签规则列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/tag-rule/list", tagRuleListGet)
		// 标签规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/tag-rule/list", ListMetaGinHandler[*TagRuleListGetSearchIn, *TagRuleListOut]())
		// 创建标签规则 - 创建新的标签识别规则（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/create", tagRuleEngineMiddleware, tagRuleCreate)
		// 创建标签规则元信息
		adminGroup.OPTIONS("/tag-rule/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagRuleCreateIn]())))
		// 标签规则详情查询 - 根据ID获取规则详细信息
		adminGroup.POST("/tag-rule/:id/get", tagRuleDetailGet)
		// 标签规则详情元信息
		adminGroup.OPTIONS("/tag-rule/:id/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[TagRuleDetailOut]())))
		// 标签规则信息更新 - 更新规则描述/内容/启用状态（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/:id/update", tagRuleEngineMiddleware, tagRuleUpdate)
		// 标签规则信息更新元信息
		adminGroup.OPTIONS("/tag-rule/:id/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagRuleUpdateIn]())))
		// 删除标签规则 - 删除规则及其全部历史版本（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/:id/delete", tagRuleEngineMiddleware, tagRuleDelete)
		// 删除标签规则元信息
		adminGroup.OPTIONS("/tag-rule/:id/delete", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 启用标签规则 - 启用指定规则（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/:id/enable", tagRuleEngineMiddleware, tagRuleSetEnabled(true))
		// 启用标签规则元信息
		adminGroup.OPTIONS("/tag-rule/:id/enable", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 停用标签规则 - 停用指定规则（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/:id/disable", tagRuleEngineMiddleware, tagRuleSetEnabled(false))
		// 停用标签规则元信息
		adminGroup.OPTIONS("/tag-rule/:id/disable", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 标签规则语法校验 - 校验 Rego 源码语法与契约，返回带行列号的错误
		adminGroup.POST("/tag-rule/validate", tagRuleValidate)
		// 标签规则语法校验元信息
		adminGroup.OPTIONS("/tag-rule/validate", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[TagRuleValidateIn](), WithBodyOutput[TagRuleValidateOut]()),
		))
		// 标签规则单规则 dry-run - 对一条候选规则评估 JSON 输入文档，输出运行细节
		adminGroup.POST("/tag-rule/single/dry-run", tagRuleDryRun)
		// 标签规则单规则 dry-run 元信息
		adminGroup.OPTIONS("/tag-rule/single/dry-run", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[TagRuleDryRunRuleIn](), WithBodyOutput[TagRuleDryRunOut]()),
		))
		// 标签规则规则组 dry-run - 对一组候选规则评估 JSON 输入文档，输出逐规则与合并标签
		adminGroup.POST("/tag-rule/group/dry-run", tagRuleDryRunGroup)
		// 标签规则规则组 dry-run 元信息
		adminGroup.OPTIONS("/tag-rule/group/dry-run", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[TagRuleDryRunGroupIn](), WithBodyOutput[TagRuleDryRunOut]()),
		))
		// 标签规则版本列表 - 获取规则的全部历史版本
		adminGroup.POST("/tag-rule/:id/version/list", tagRuleVersionList)
		// 标签规则版本列表元信息
		adminGroup.OPTIONS("/tag-rule/:id/version/list", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 标签规则回滚 - 将规则回滚到指定历史版本（需要标签引擎中间件）
		adminGroup.POST("/tag-rule/:id/version/:version/rollback", tagRuleEngineMiddleware, tagRuleVersionRollback)
		// 标签规则回滚元信息
		adminGroup.OPTIONS("/tag-rule/:id/version/:version/rollback", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 标签规则目标模型选项 - 返回已注册目标模型的下拉选项列表
		adminGroup.POST("/tag-rule/model/options", tagRuleModelOptions)
		// 标签规则目标模型选项元信息
		adminGroup.OPTIONS("/tag-rule/model/options", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[TagRuleModelOptionsOut]())))
		// 标签规则目标模型元信息 - 返回输入文档字段元信息，供 Rego 编辑器补全与字段面板
		adminGroup.POST("/tag-rule/model/metadata", tagRuleModelMetadata)
		// 标签规则目标模型元信息元信息
		adminGroup.OPTIONS("/tag-rule/model/metadata", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagRuleModelIn](), WithBodyOutput[TagRuleModelMetadataOut]())))
		// 标签规则目标模型样例 - 返回系统样例列表，缺失时懒供给
		adminGroup.POST("/tag-rule/model/samples", tagRuleModelSamples)
		// 标签规则目标模型样例元信息
		adminGroup.OPTIONS("/tag-rule/model/samples", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagRuleModelIn](), WithBodyOutput[TagRuleModelSamplesOut]())))

		// ========== 标签事件规则管理接口 ==========
		// 提供标签事件规则的增删改查、启停、表达式校验、dry-run 与版本回滚

		// 标签事件规则列表查询 - 支持分页、搜索和排序
		adminGroup.POST("/tag-event-rule/list", tagEventRuleListGet)
		// 标签事件规则列表自省信息 - 返回搜索选项、表格列与各 HTTP 方法的入参出参结构体定义
		adminGroup.OPTIONS("/tag-event-rule/list", ListMetaGinHandler[*TagEventRuleListGetSearchIn, *TagEventRuleListOut]())
		// 创建标签事件规则 - 创建新的标签事件规则，创建前逐条件校验表达式语法
		adminGroup.POST("/tag-event-rule/create", tagEventRuleCreate)
		// 创建标签事件规则元信息
		adminGroup.OPTIONS("/tag-event-rule/create", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagEventRuleCreateIn]())))
		// 标签事件规则详情查询 - 根据ID获取规则详细信息
		adminGroup.POST("/tag-event-rule/:id/get", tagEventRuleDetailGet)
		// 标签事件规则详情元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[TagEventRuleDetailOut]())))
		// 标签事件规则信息更新 - 更新规则描述/匹配策略/启用状态/条件集合，更新前快照当前版本
		adminGroup.POST("/tag-event-rule/:id/update", tagEventRuleUpdate)
		// 标签事件规则信息更新元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/update", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[TagEventRuleUpdateIn]())))
		// 删除标签事件规则 - 删除规则及其全部条件与历史版本
		adminGroup.POST("/tag-event-rule/:id/delete", tagEventRuleDelete)
		// 删除标签事件规则元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/delete", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 启用标签事件规则 - 启用指定规则
		adminGroup.POST("/tag-event-rule/:id/enable", tagEventRuleSetEnabled(true))
		// 启用标签事件规则元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/enable", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 停用标签事件规则 - 停用指定规则
		adminGroup.POST("/tag-event-rule/:id/disable", tagEventRuleSetEnabled(false))
		// 停用标签事件规则元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/disable", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 标签事件规则表达式校验 - 校验条件表达式语法，返回精炼后的标准表达式或错误消息
		adminGroup.POST("/tag-event-rule/validate", tagEventRuleValidate)
		// 标签事件规则表达式校验元信息
		adminGroup.OPTIONS("/tag-event-rule/validate", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[TagEventRuleValidateIn](), WithBodyOutput[TagEventRuleValidateOut]()),
		))
		// 标签事件规则 dry-run - 对一条候选规则（不落库）执行事件检测，需 raw 库中间件查询 tag_summings
		adminGroup.POST("/tag-event-rule/dry-run", rawDatabaseMiddleware, tagEventRuleDryRun)
		// 标签事件规则 dry-run 元信息
		adminGroup.OPTIONS("/tag-event-rule/dry-run", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[TagEventRuleDryRunIn](), WithBodyOutput[TagEventRuleDryRunOut]()),
		))
		// 标签事件规则版本列表 - 获取规则的全部历史版本
		adminGroup.POST("/tag-event-rule/:id/version/list", tagEventRuleVersionList)
		// 标签事件规则版本列表元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/version/list", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 标签事件规则回滚 - 将规则回滚到指定历史版本，回滚前先快照当前内容
		adminGroup.POST("/tag-event-rule/:id/version/:version/rollback", tagEventRuleVersionRollback)
		// 标签事件规则回滚元信息
		adminGroup.OPTIONS("/tag-event-rule/:id/version/:version/rollback", StructMetaGinHandler(WithMethod(http.MethodPost)))
	}

	// ==================== 原始流量审计接口组：/api/v1/traffic ====================
	// 只读查询 raw 库（ClickHouse）中的原始流量数据。
	// 三种数据类型（ipv4-tcp / ipv6-tcp / http-transport）的数据结构与业务逻辑差异较大，
	// 拆分为三组独立路由各自演进；认证 / RBAC / raw 库 / S3 / 命名空间等公共中间件挂在父组上继承。
	// 每组仅提供：
	//   - POST /list      分页查询流量全字段（payload/body 已转存 S3，单条返回字节量可控）
	//
	// 三种流量数据均无唯一性 ID，条件查询无法可靠定位单条记录，故不提供 detail 接口；
	// 加密字段以 base64 密文、S3 body 以对象键形式返回，
	// 明文查看走全局通用接口 /api/v1/plaintext/crypto 与 /api/v1/plaintext/s3（见下方明文组）。

	trafficGroup := v1Group.Group(
		"/traffic",                  // 流量审计接口路径前缀
		authenticatorMiddleware,     // 认证器注入
		forceAuthenticateMiddleware, // 强制JWT认证
		policyMiddleware,            // RBAC策略引擎
		rawDatabaseMiddleware,       // raw 库（ClickHouse）连接
		s3Middleware,                // S3 客户端（body 明文读取）
		namespaceMiddleware,         // 命名空间隔离
	)

	// ========== IPv4 TCP 包：/api/v1/traffic/ipv4-tcp ==========
	{
		ipv4TcpGroup := trafficGroup.Group("/ipv4-tcp")
		// 流量列表查询 - 分页查询 IPv4 TCP 包全字段
		ipv4TcpGroup.POST("/list", ipv4TcpListGet)
		// 流量列表元信息
		ipv4TcpGroup.OPTIONS("/list", ListMetaGinHandler[*Ipv4TcpListGetSearchIn, *Ipv4TcpListGetListItemOut]())
	}

	// ========== IPv6 TCP 包：/api/v1/traffic/ipv6-tcp ==========
	{
		ipv6TcpGroup := trafficGroup.Group("/ipv6-tcp")
		// 流量列表查询 - 分页查询 IPv6 TCP 包全字段
		ipv6TcpGroup.POST("/list", ipv6TcpListGet)
		// 流量列表元信息
		ipv6TcpGroup.OPTIONS("/list", ListMetaGinHandler[*Ipv6TcpListGetSearchIn, *Ipv6TcpListGetListItemOut]())
	}

	// ========== HTTP 传输：/api/v1/traffic/http-transport ==========
	{
		httpTransportGroup := trafficGroup.Group("/http-transport")
		// 流量列表查询 - 分页查询 HTTP 传输全字段
		httpTransportGroup.POST("/list", httpTransportListGet)
		// 流量列表元信息
		httpTransportGroup.OPTIONS("/list", ListMetaGinHandler[*HttpTransportListGetSearchIn, *HttpTransportListGetListItemOut]())
	}

	// ==================== 通用明文查看接口组：/api/v1/plaintext ====================
	// 全局通用接口：解密加密字段密文 / 读取 S3 body 明文，响应原始字节流。
	// 权限点 object = plaintext:{crypto|s3}:{model}:{field}，action = READ（规范见 internal/pkg/base/consts.go）。
	// 新增模型的加密/S3 字段：RegisterPlaintextModel 注册模型 + 补模板种子即可，无需新增接口。
	plaintextGroup := v1Group.Group(
		"/plaintext",                // 明文查看接口路径前缀
		authenticatorMiddleware,     // 认证器注入
		forceAuthenticateMiddleware, // 强制JWT认证
		policyMiddleware,            // RBAC策略引擎
		s3Middleware,                // S3 客户端（S3 body 明文读取）
		namespaceMiddleware,         // 命名空间隔离
	)
	// 加密字段明文解密 - 请求携带模型注册名 + 字段 + base64 密文，按字段权限点静态授权后返回明文
	plaintextGroup.POST("/crypto", manager.CryptoPlaintextGet)
	// 加密字段明文解密元信息
	plaintextGroup.OPTIONS("/crypto", StructMetaGinHandler(
		WithMethod(http.MethodPost, WithBodyInput[manager.CryptoPlaintextIn]()),
	))
	// S3 body 明文读取 - 请求携带模型注册名 + 字段 + 对象键，按字段权限点静态授权后返回明文
	plaintextGroup.POST("/s3", manager.S3PlaintextGet)
	// S3 body 明文读取元信息
	plaintextGroup.OPTIONS("/s3", StructMetaGinHandler(
		WithMethod(http.MethodPost, WithBodyInput[manager.S3PlaintextIn]()),
	))

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
			coreDatabaseMiddleware, // 数据库连接，用于创建用户记录
			userRegister,           // 用户注册处理函数
		)
		// 用户注册元信息
		userGroup.OPTIONS("/register", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserRegisterIn]())))
		// 密码登录 - 使用用户名密码进行登录认证
		userGroup.POST(
			"/password/login",
			authenticatorMiddleware, // 认证器注入，用于生成JWT令牌
			coreDatabaseMiddleware,  // 数据库连接，用于验证用户凭证
			userPasswordLogin,       // 密码登录处理函数
		)
		// 密码登录元信息
		userGroup.OPTIONS("/password/login", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[UserPasswordLoginIn](), WithBodyOutput[UserTokenOut]()),
		))
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
			coreDatabaseMiddleware,      // 数据库连接，用于更新密码
			userSelfPasswordReset,       // 密码重置处理函数
		)
		// 密码重置元信息
		selfGroup.OPTIONS("/password/reset", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserPasswordResetIn]())))
		// JWT令牌刷新 - 刷新过期的JWT访问令牌
		selfGroup.POST(
			"/jwt/refresh",
			authenticatorMiddleware, // 认证器注入，用于生成新令牌
			userSelfJwtRefresh,      // JWT刷新处理函数
		)
		// JWT令牌刷新元信息
		selfGroup.OPTIONS("/jwt/refresh", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[UserTokenOut]())))
		// 授权信息获取 - 获取用户的RBAC权限和可访问的命名空间
		selfGroup.POST(
			"/authorization-info/get",
			authenticatorMiddleware,      // 认证器注入
			forceAuthenticateMiddleware,  // 强制认证
			coreDatabaseMiddleware,       // 数据库连接，查询权限信息
			userSelfAuthorizationInfoGet, // 授权信息获取处理函数
		)
		// 授权信息获取元信息
		selfGroup.OPTIONS("/authorization-info/get", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[AuthorizationInfoOut]())))
		// 个人资料修改 - 更新用户的昵称、邮箱等个人信息
		selfGroup.POST(
			"/profile/change",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，更新用户信息
			userSelfProfileChange,       // 个人资料修改处理函数
		)
		// 个人资料修改元信息
		selfGroup.OPTIONS("/profile/change", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[UserProfileChangeIn]())))
		// 访问密钥列表获取 - 获取用户的所有访问密钥
		selfGroup.POST(
			"/access-key/list",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，查询访问密钥
			userSelfAccessKeyListGet,    // 访问密钥列表获取处理函数
		)
		// 访问密钥列表元信息
		selfGroup.OPTIONS("/access-key/list", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[UserAccessKeyOut]())))
		// 创建访问密钥 - 为用户创建新的访问密钥
		selfGroup.POST(
			"/access-key/create",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，创建访问密钥
			userSelfAccessKeyCreate,     // 访问密钥创建处理函数
		)
		// 创建访问密钥元信息
		selfGroup.OPTIONS("/access-key/create", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[UserAccessKeyCreateIn](), WithBodyOutput[UserAccessKeyOut]()),
		))
		// 删除访问密钥 - 删除指定的访问密钥
		selfGroup.POST(
			"/access-key/:id/remove",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，删除访问密钥
			userSelfAccessKeyRemove,     // 访问密钥删除处理函数
		)
		// 删除访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/remove", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 激活访问密钥 - 启用已禁用的访问密钥
		selfGroup.POST(
			"/access-key/:id/activate",
			authenticatorMiddleware,        // 认证器注入
			forceAuthenticateMiddleware,    // 强制认证
			coreDatabaseMiddleware,         // 数据库连接，更新密钥状态
			userSelfAccessKeyEnabled(true), // 访问密钥启用处理函数
		)
		// 激活访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/activate", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[UserAccessKeyOut]())))
		// 停用访问密钥 - 禁用正在使用的访问密钥
		selfGroup.POST(
			"/access-key/:id/deactivate",
			authenticatorMiddleware,         // 认证器注入
			forceAuthenticateMiddleware,     // 强制认证
			coreDatabaseMiddleware,          // 数据库连接，更新密钥状态
			userSelfAccessKeyEnabled(false), // 访问密钥停用处理函数
		)
		// 停用访问密钥元信息
		selfGroup.OPTIONS("/access-key/:id/deactivate", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[UserAccessKeyOut]())))
		// 前端配置查询 - 获取当前用户保存的前端页面配置
		selfGroup.POST(
			"/frontend-config/get",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，查询配置
			userSelfFrontendConfigGet,   // 前端配置查询处理函数
		)
		// 前端配置查询元信息
		selfGroup.OPTIONS("/frontend-config/get", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyOutput[UserFrontendConfigOut]()),
		))
		// 前端配置同步 - 上传当前用户的前端页面配置（乐观锁并发校验）
		selfGroup.POST(
			"/frontend-config/sync",
			authenticatorMiddleware,     // 认证器注入
			forceAuthenticateMiddleware, // 强制认证
			coreDatabaseMiddleware,      // 数据库连接，保存配置
			userSelfFrontendConfigSync,  // 前端配置同步处理函数
		)
		// 前端配置同步元信息
		selfGroup.OPTIONS("/frontend-config/sync", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[UserFrontendConfigSyncIn](), WithBodyOutput[UserFrontendConfigOut]()),
		))
		// 前端配置同步开关 - 设置当前用户是否开启前端配置同步
		selfGroup.POST(
			"/frontend-config/sync-enabled/set",
			authenticatorMiddleware,              // 认证器注入
			forceAuthenticateMiddleware,          // 强制认证
			coreDatabaseMiddleware,               // 数据库连接，更新开关
			userSelfFrontendConfigSyncEnabledSet, // 同步开关设置处理函数
		)
		// 前端配置同步开关元信息
		selfGroup.OPTIONS("/frontend-config/sync-enabled/set", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[UserFrontendConfigSyncEnabledIn]()),
		))
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
		// 服务器列表元信息
		inspectorGroup.OPTIONS("/server/list", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[PipelineServerInfo]())))
		// 调度器列表查询 - 获取所有任务调度器信息
		inspectorGroup.POST("/scheduler/list", etlPipelineInspectorSchedulerListGet)
		// 调度器列表元信息
		inspectorGroup.OPTIONS("/scheduler/list", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyOutput[PipelineSchedulerInfo]())))
		// 队列列表查询 - 获取所有任务队列信息
		inspectorGroup.POST("/queue/list", etlPipelineInspectorQueueListGet)
		// 队列列表元信息
		inspectorGroup.OPTIONS("/queue/list", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 队列恢复运行 - 恢复指定队列的任务处理
		inspectorGroup.POST("/queue/:name/unpause", etlPipelineInspectorQueueUnpause)
		// 队列恢复运行元信息
		inspectorGroup.OPTIONS("/queue/:name/unpause", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 队列暂停 - 暂停指定队列的任务处理
		inspectorGroup.POST("/queue/:name/pause", etlPipelineInspectorQueuePause)
		// 队列暂停元信息
		inspectorGroup.OPTIONS("/queue/:name/pause", StructMetaGinHandler(WithMethod(http.MethodPost)))
		// 队列每日统计查询 - 获取队列的每日任务统计信息
		inspectorGroup.POST("/queue/daily-stat/list", etlPipelineInspectorDailyStatListGet)
		// 队列每日统计元信息
		inspectorGroup.OPTIONS("/queue/daily-stat/list", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorDailyStatIn]())))
		// 任务列表查询 - 获取所有任务信息
		inspectorGroup.POST("/task/list", etlPipelineInspectorTaskListGet)
		// 任务列表元信息
		inspectorGroup.OPTIONS("/task/list", StructMetaGinHandler(
			WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorTaskListIn](), WithBodyOutput[PipelineTaskInfo]()),
		))
		// 按状态删除任务 - 删除指定状态的所有任务
		inspectorGroup.POST("/task/state/delete", etlPipelineInspectorStateTaskDelete)
		// 按状态删除任务元信息
		inspectorGroup.OPTIONS("/task/state/delete", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorStateTaskDeleteIn]())))
		// 按状态归档任务 - 归档指定状态的所有任务
		inspectorGroup.POST("/task/state/archive", etlPipelineInspectorStateTaskArchive)
		// 按状态归档任务元信息
		inspectorGroup.OPTIONS("/task/state/archive", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorStateTaskDeleteIn]())))
		// 按状态运行任务 - 运行指定状态的所有任务
		inspectorGroup.POST("/task/state/run", etlPipelineInspectorStateTaskRun)
		// 按状态运行任务元信息
		inspectorGroup.OPTIONS("/task/state/run", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorStateTaskRunIn]())))
		// 删除单个任务 - 删除指定ID的任务
		inspectorGroup.POST("/task/:id/delete", etlPipelineInspectorTaskDelete)
		// 删除单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/delete", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorSingleTaskIn]())))
		// 归档单个任务 - 归档指定ID的任务
		inspectorGroup.POST("/task/:id/archive", etlPipelineInspectorTaskArchive)
		// 归档单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/archive", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorSingleTaskIn]())))
		// 运行单个任务 - 运行指定ID的任务
		inspectorGroup.POST("/task/:id/run", etlPipelineInspectorTaskRun)
		// 运行单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/run", StructMetaGinHandler(WithMethod(http.MethodPost, WithBodyInput[ETLPipelineInspectorSingleTaskIn]())))
		// 取消处理中的任务 - 取消正在处理的任务
		inspectorGroup.POST("/task/:id/cancel-processing", etlPipelineInspectorTaskCancelPorcessing)
		// 取消处理中的任务元信息
		inspectorGroup.OPTIONS("/task/:id/cancel-processing", StructMetaGinHandler(WithMethod(http.MethodPost)))
	}
	return nil
}

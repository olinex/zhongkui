package router

import (
	"net/http"

	"zhongkui.com/zhongkui/internal/manager/views"
)

// registerEtlRoutes 注册 /api/v1/etl ETL管道监控接口路由。
// 提供ETL（Extract-Transform-Load）任务管道的监控和管理功能。
func registerEtlRoutes(ctx *RegisterContext) {
	// 创建ETL接口组
	// ETL 为全局资源（无命名空间隔离需求），认证链挂载后命名空间中间件
	// 在无请求头/查询参数/Cookie 时回落默认命名空间 "default"，授权按该命名空间执行。
	etlGroup := ctx.v1Group.Group(
		"/etl",                          // ETL 接口路径前缀
		ctx.authenticatorMiddleware,     // 认证器注入
		ctx.forceAuthenticateMiddleware, // 强制JWT认证
		ctx.policyMiddleware,            // RBAC策略引擎
		ctx.namespaceMiddleware,         // 命名空间隔离（回落 default）
		ctx.authorizeMiddleware,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
	)
	{
		// ========== 检查器子组：/api/v1/etl/inspector ==========
		// 提供异步任务队列的监控和管理，需要管道检查器中间件

		// 创建检查器子组，添加管道检查器与 etcd 客户端中间件
		inspectorGroup := etlGroup.Group("/inspector", ctx.pipelineInspectorMiddleware, ctx.etcdMiddleware)

		// 服务器列表查询 - 获取所有任务队列服务器信息
		inspectorGroup.POST("/server/list", views.EtlPipelineInspectorServerListGet)
		// 服务器列表元信息
		inspectorGroup.OPTIONS("/server/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.PipelineServerInfo]())))
		// 调度器列表查询 - 获取所有任务调度器信息
		inspectorGroup.POST("/scheduler/list", views.EtlPipelineInspectorSchedulerListGet)
		// 调度器列表元信息
		inspectorGroup.OPTIONS("/scheduler/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyOutput[views.PipelineSchedulerInfo]())))
		// 队列列表查询 - 获取所有任务队列信息
		inspectorGroup.POST("/queue/list", views.EtlPipelineInspectorQueueListGet)
		// 队列列表元信息
		inspectorGroup.OPTIONS("/queue/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 队列恢复运行 - 恢复指定队列的任务处理
		inspectorGroup.POST("/queue/:name/unpause", views.EtlPipelineInspectorQueueUnpause)
		// 队列恢复运行元信息
		inspectorGroup.OPTIONS("/queue/:name/unpause", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 队列暂停 - 暂停指定队列的任务处理
		inspectorGroup.POST("/queue/:name/pause", views.EtlPipelineInspectorQueuePause)
		// 队列暂停元信息
		inspectorGroup.OPTIONS("/queue/:name/pause", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
		// 队列每日统计查询 - 获取队列的每日任务统计信息
		inspectorGroup.POST("/queue/daily-stat/list", views.EtlPipelineInspectorDailyStatListGet)
		// 队列每日统计元信息
		inspectorGroup.OPTIONS("/queue/daily-stat/list", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorDailyStatIn]())))
		// 任务列表查询 - 获取所有任务信息
		inspectorGroup.POST("/task/list", views.EtlPipelineInspectorTaskListGet)
		// 任务列表元信息
		inspectorGroup.OPTIONS("/task/list", views.StructMetaGinHandler(
			views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorTaskListIn](), views.WithBodyOutput[views.PipelineTaskInfo]()),
		))
		// 按状态删除任务 - 删除指定状态的所有任务
		inspectorGroup.POST("/task/state/delete", views.EtlPipelineInspectorStateTaskDelete)
		// 按状态删除任务元信息
		inspectorGroup.OPTIONS("/task/state/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorStateTaskDeleteIn]())))
		// 按状态归档任务 - 归档指定状态的所有任务
		inspectorGroup.POST("/task/state/archive", views.EtlPipelineInspectorStateTaskArchive)
		// 按状态归档任务元信息
		inspectorGroup.OPTIONS("/task/state/archive", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorStateTaskDeleteIn]())))
		// 按状态运行任务 - 运行指定状态的所有任务
		inspectorGroup.POST("/task/state/run", views.EtlPipelineInspectorStateTaskRun)
		// 按状态运行任务元信息
		inspectorGroup.OPTIONS("/task/state/run", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorStateTaskRunIn]())))
		// 删除单个任务 - 删除指定ID的任务
		inspectorGroup.POST("/task/:id/delete", views.EtlPipelineInspectorTaskDelete)
		// 删除单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorSingleTaskIn]())))
		// 归档单个任务 - 归档指定ID的任务
		inspectorGroup.POST("/task/:id/archive", views.EtlPipelineInspectorTaskArchive)
		// 归档单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/archive", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorSingleTaskIn]())))
		// 运行单个任务 - 运行指定ID的任务
		inspectorGroup.POST("/task/:id/run", views.EtlPipelineInspectorTaskRun)
		// 运行单个任务元信息
		inspectorGroup.OPTIONS("/task/:id/run", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.ETLPipelineInspectorSingleTaskIn]())))
		// 取消处理中的任务 - 取消正在处理的任务
		inspectorGroup.POST("/task/:id/cancel-processing", views.EtlPipelineInspectorTaskCancelPorcessing)
		// 取消处理中的任务元信息
		inspectorGroup.OPTIONS("/task/:id/cancel-processing", views.StructMetaGinHandler(views.WithMethod(http.MethodPost)))
	}

	// ========== 定时任务管理子组：/api/v1/etl/scheduler ==========
	// 提供定时任务(cron/interval)的增删改启停与"立即执行"，读写 etcd 配置。

	// 创建定时任务管理子组，添加 etcd 客户端与管道投递客户端中间件
	schedulerGroup := etlGroup.Group("/scheduler", ctx.etcdMiddleware, ctx.pipelineClientMiddleware)
	{
		// 创建定时任务
		schedulerGroup.POST("/task/create", views.EtlSchedulerTaskCreate)
		// 创建定时任务元信息
		schedulerGroup.OPTIONS("/task/create", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.EtlSchedulerTaskIn]())))
		// 更新定时任务
		schedulerGroup.POST("/task/update", views.EtlSchedulerTaskUpdate)
		// 更新定时任务元信息
		schedulerGroup.OPTIONS("/task/update", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.EtlSchedulerTaskIn]())))
		// 切换定时任务启用状态
		schedulerGroup.POST("/task/toggle", views.EtlSchedulerTaskToggle)
		// 切换定时任务启用状态元信息
		schedulerGroup.OPTIONS("/task/toggle", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.EtlSchedulerTaskToggleIn]())))
		// 删除定时任务
		schedulerGroup.POST("/task/delete", views.EtlSchedulerTaskDelete)
		// 删除定时任务元信息
		schedulerGroup.OPTIONS("/task/delete", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.EtlSchedulerTaskKeyIn]())))
		// 立即执行定时任务
		schedulerGroup.POST("/task/run", views.EtlSchedulerTaskRun)
		// 立即执行定时任务元信息
		schedulerGroup.OPTIONS("/task/run", views.StructMetaGinHandler(views.WithMethod(http.MethodPost, views.WithBodyInput[views.EtlSchedulerTaskKeyIn]())))
	}
}

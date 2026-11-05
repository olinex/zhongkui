package etl

import (
	"net/http"

	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	// blank import：触发 worker 包 init 注册 taskType 枚举类（取值空间为任务处理器注册表），
	// 供任务列表的任务类型筛选与 /admin/enum 自省接口使用。
	_ "zhongkuisec.com/zhongkui/internal/worker"
)

// RegisterRoutes 注册 /api/v1/etl ETL管道监控接口路由。
// 提供ETL（Extract-Transform-Load）任务管道的监控和管理功能。
func RegisterRoutes(ctx *views.RegisterContext) {
	// 创建ETL接口组
	// ETL 为全局资源（无命名空间隔离需求），认证链挂载后命名空间中间件
	// 在无请求头/查询参数/Cookie 时回落默认命名空间 "default"，授权按该命名空间执行。
	etlGroup := ctx.V1Group.Group(
		"/etl",                // ETL 接口路径前缀
		ctx.Authenticator,     // 认证器注入
		ctx.ForceAuthenticate, // 强制JWT认证
		ctx.Policy,            // RBAC策略引擎
		ctx.Namespace,         // 命名空间隔离（回落 default）
		ctx.Authorize,         // 权限验证（api:{FullPath} + HTTP 方法动态授权）
	)
	{
		// ========== 检查器子组：/api/v1/etl/inspector ==========
		// 提供异步任务队列的监控和管理，需要管道检查器中间件

		// 创建检查器子组，添加管道检查器与 etcd 客户端中间件
		inspectorGroup := etlGroup.Group("/inspector", ctx.PipelineInspector, ctx.Etcd)

		// 服务器列表查询 - 获取所有任务队列服务器信息
		inspectorGroup.POST("/server/list", EtlPipelineInspectorServerListGet)
		// 服务器列表元信息
		views.RegisterApi(inspectorGroup, "/server/list", "服务器列表查询 - 获取所有任务队列服务器信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[PipelineServerInfo]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "etl.server.list",
				Domain:    "etl",
				Intent:    "查看异步任务服务器列表",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 调度器列表查询 - 获取所有任务调度器信息
		inspectorGroup.POST("/scheduler/list", EtlPipelineInspectorSchedulerListGet)
		// 调度器列表元信息
		views.RegisterApi(inspectorGroup, "/scheduler/list", "调度器列表查询 - 获取所有任务调度器信息",
			views.WithMethod(http.MethodPost, views.WithBodyOutput[PipelineSchedulerInfo]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "etl.scheduler.list",
				Domain:    "etl",
				Intent:    "查看任务调度器列表",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 队列列表查询 - 获取所有任务队列信息
		inspectorGroup.POST("/queue/list", EtlPipelineInspectorQueueListGet)
		// 队列列表元信息
		views.RegisterApi(inspectorGroup, "/queue/list", "队列列表查询 - 获取所有任务队列信息",
			views.WithMethod(http.MethodPost),
			views.WithCapability(views.CapabilityMeta{
				ID:        "etl.queue.list",
				Domain:    "etl",
				Intent:    "查看任务队列列表",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 队列恢复运行 - 恢复指定队列的任务处理
		inspectorGroup.POST("/queue/:name/unpause", EtlPipelineInspectorQueueUnpause)
		// 队列恢复运行元信息
		views.RegisterApi(inspectorGroup, "/queue/:name/unpause", "队列恢复运行 - 恢复指定队列的任务处理", views.WithMethod(http.MethodPost))
		// 队列暂停 - 暂停指定队列的任务处理
		inspectorGroup.POST("/queue/:name/pause", EtlPipelineInspectorQueuePause)
		// 队列暂停元信息
		views.RegisterApi(inspectorGroup, "/queue/:name/pause", "队列暂停 - 暂停指定队列的任务处理", views.WithMethod(http.MethodPost))
		// 队列每日统计查询 - 获取队列的每日任务统计信息
		inspectorGroup.POST("/queue/daily-stat/list", EtlPipelineInspectorDailyStatListGet)
		// 队列每日统计元信息
		views.RegisterApi(inspectorGroup, "/queue/daily-stat/list", "队列每日统计查询 - 获取队列的每日任务统计信息",
			views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorDailyStatIn]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "etl.queue.daily-stat",
				Domain:    "etl",
				Intent:    "查看队列每日任务统计",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 任务列表查询 - 获取所有任务信息
		inspectorGroup.POST("/task/list", EtlPipelineInspectorTaskListGet)
		// 任务列表元信息
		views.RegisterApi(inspectorGroup, "/task/list", "任务列表查询 - 获取所有任务信息",
			views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorTaskListIn](), views.WithBodyOutput[ETLPipelineInspectorTaskListOut]()),
			views.WithCapability(views.CapabilityMeta{
				ID:        "etl.task.list",
				Domain:    "etl",
				Intent:    "查看异步任务列表",
				Entities:  []string{},
				Access:    capability.AccessRead,
				Invokable: true,
			}),
		)
		// 按状态删除任务 - 删除指定状态的所有任务
		inspectorGroup.POST("/task/state/delete", EtlPipelineInspectorStateTaskDelete)
		// 按状态删除任务元信息
		views.RegisterApi(inspectorGroup, "/task/state/delete", "按状态删除任务 - 删除指定状态的所有任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorStateTaskDeleteIn]()))
		// 按状态归档任务 - 归档指定状态的所有任务
		inspectorGroup.POST("/task/state/archive", EtlPipelineInspectorStateTaskArchive)
		// 按状态归档任务元信息
		views.RegisterApi(inspectorGroup, "/task/state/archive", "按状态归档任务 - 归档指定状态的所有任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorStateTaskDeleteIn]()))
		// 按状态运行任务 - 运行指定状态的所有任务
		inspectorGroup.POST("/task/state/run", EtlPipelineInspectorStateTaskRun)
		// 按状态运行任务元信息
		views.RegisterApi(inspectorGroup, "/task/state/run", "按状态运行任务 - 运行指定状态的所有任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorStateTaskRunIn]()))
		// 删除单个任务 - 删除指定ID的任务
		inspectorGroup.POST("/task/:id/delete", EtlPipelineInspectorTaskDelete)
		// 删除单个任务元信息
		views.RegisterApi(inspectorGroup, "/task/:id/delete", "删除单个任务 - 删除指定ID的任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorSingleTaskIn]()))
		// 归档单个任务 - 归档指定ID的任务
		inspectorGroup.POST("/task/:id/archive", EtlPipelineInspectorTaskArchive)
		// 归档单个任务元信息
		views.RegisterApi(inspectorGroup, "/task/:id/archive", "归档单个任务 - 归档指定ID的任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorSingleTaskIn]()))
		// 运行单个任务 - 运行指定ID的任务
		inspectorGroup.POST("/task/:id/run", EtlPipelineInspectorTaskRun)
		// 运行单个任务元信息
		views.RegisterApi(inspectorGroup, "/task/:id/run", "运行单个任务 - 运行指定ID的任务", views.WithMethod(http.MethodPost, views.WithBodyInput[ETLPipelineInspectorSingleTaskIn]()))
		// 取消处理中的任务 - 取消正在处理的任务
		inspectorGroup.POST("/task/:id/cancel-processing", EtlPipelineInspectorTaskCancelPorcessing)
		// 取消处理中的任务元信息
		views.RegisterApi(inspectorGroup, "/task/:id/cancel-processing", "取消处理中的任务 - 取消正在处理的任务", views.WithMethod(http.MethodPost))
	}

	// ========== 定时任务管理子组：/api/v1/etl/scheduler ==========
	// 提供定时任务(cron/interval)的增删改启停与"立即执行"，读写 etcd 配置。

	// 创建定时任务管理子组，添加 etcd 客户端与管道投递客户端中间件
	schedulerGroup := etlGroup.Group("/scheduler", ctx.Etcd, ctx.PipelineClient)
	{
		// 创建定时任务
		schedulerGroup.POST("/task/create", EtlSchedulerTaskCreate)
		// 创建定时任务元信息
		views.RegisterApi(schedulerGroup, "/task/create", "创建定时任务", views.WithMethod(http.MethodPost, views.WithBodyInput[EtlSchedulerTaskIn]()))
		// 更新定时任务
		schedulerGroup.POST("/task/update", EtlSchedulerTaskUpdate)
		// 更新定时任务元信息
		views.RegisterApi(schedulerGroup, "/task/update", "更新定时任务", views.WithMethod(http.MethodPost, views.WithBodyInput[EtlSchedulerTaskIn]()))
		// 切换定时任务启用状态
		schedulerGroup.POST("/task/toggle", EtlSchedulerTaskToggle)
		// 切换定时任务启用状态元信息
		views.RegisterApi(schedulerGroup, "/task/toggle", "切换定时任务启用状态", views.WithMethod(http.MethodPost, views.WithBodyInput[EtlSchedulerTaskToggleIn]()))
		// 删除定时任务
		schedulerGroup.POST("/task/delete", EtlSchedulerTaskDelete)
		// 删除定时任务元信息
		views.RegisterApi(schedulerGroup, "/task/delete", "删除定时任务", views.WithMethod(http.MethodPost, views.WithBodyInput[EtlSchedulerTaskKeyIn]()))
		// 立即执行定时任务
		schedulerGroup.POST("/task/run", EtlSchedulerTaskRun)
		// 立即执行定时任务元信息
		views.RegisterApi(schedulerGroup, "/task/run", "立即执行定时任务", views.WithMethod(http.MethodPost, views.WithBodyInput[EtlSchedulerTaskKeyIn]()))
	}
}

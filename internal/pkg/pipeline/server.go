// Package pipeline 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。
package pipeline

import (
	"github.com/hibiken/asynq"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

// HandlerFunc 是 asynq.HandlerFunc 的类型别名，表示任务处理函数。
// 该函数类型用于处理从任务队列中接收到的任务。
// 函数签名：func(ctx context.Context, task *asynq.Task) error
// 参数：
//   - ctx: context.Context - 任务执行上下文，包含超时、取消等控制信息
//   - task: *asynq.Task - 任务对象，包含任务类型和负载数据
//
// 返回值：
//   - error - 任务执行结果，nil 表示成功，非 nil 表示失败
//
// 使用场景：
//   - 处理异步任务，如发送邮件、生成报告、处理图片等
//   - 实现分布式任务处理系统
//   - 构建后台作业队列
type HandlerFunc = asynq.HandlerFunc

// HandlerFuncs 是任务处理函数的映射表类型别名。
// 该映射表用于将任务类型（模式）映射到对应的处理函数。
// 键：string - 任务类型/模式，用于匹配队列中的任务类型
// 值：HandlerFunc - 对应的任务处理函数
//
// 示例：
//
//	hFuncs := HandlerFuncs{
//	    "email:send":      sendEmailHandler,
//	    "report:generate": generateReportHandler,
//	    "image:process":   processImageHandler,
//	}
//
// 注意事项：
//  1. 任务类型应该使用有意义的命名，建议使用 "domain:action" 格式
//  2. 同一个任务类型只能有一个处理函数
//  3. 处理函数应该幂等，因为任务可能会被重试
type HandlerFuncs = map[string]HandlerFunc

// RawConfig 是 asynq.Config 的类型别名，表示 asynq 服务器的原始配置。
// 该配置包含任务队列服务器的各种参数，如并发数、重试策略、队列配置等。
// 详细配置选项请参考 asynq 官方文档：https://github.com/hibiken/asynq
//
// 主要配置项：
//   - Concurrency: 并发处理任务的数量
//   - Queues: 队列优先级映射
//   - RetryDelayFunc: 任务重试延迟函数
//   - Logger: 日志记录器
//   - LogLevel: 日志级别
//
// 使用方式：
//
//	cfg := asynq.Config{
//	    Concurrency: 10,
//	    Queues: map[string]int{
//	        "critical": 6,
//	        "default":  3,
//	        "low":      1,
//	    },
//	}
type RawConfig = asynq.Config

// Server 表示任务队列服务器，负责从 Redis 队列中获取任务并分发给对应的处理函数。
// 该结构体封装了 asynq 服务器的核心功能，提供启动、停止和任务处理能力。
//
// 字段说明：
//   - logger: *logging.Logger - 日志记录器，用于记录服务器运行状态和错误信息
//   - srv: *asynq.Server - asynq 服务器实例，负责与 Redis 交互和任务调度
//   - mux: *asynq.ServeMux - 任务分发器，根据任务类型路由到对应的处理函数
//
// 生命周期：
//  1. 通过 NewServer 函数创建服务器实例
//  2. 调用 Start 方法启动服务器，开始处理任务
//  3. 调用 Shutdown 方法优雅关闭服务器
//
// 线程安全性：
//   - Server 实例是线程安全的，可以在多个 goroutine 中安全使用
//   - Start 和 Shutdown 方法应该由同一个 goroutine 调用
type Server struct {
	logger *logging.Logger // 日志记录器，用于记录服务器运行状态和错误信息
	srv    *asynq.Server   // asynq 服务器实例，负责与 Redis 交互和任务调度
	mux    *asynq.ServeMux // 任务分发器，根据任务类型路由到对应的处理函数
}

// NewServer 创建一个新的任务队列服务器实例。
// 该函数初始化服务器并配置所有的任务处理函数。
//
// 参数：
//   - cli: *redis.Client - Redis 客户端实例，用于连接任务队列存储
//   - cfg: RawConfig - asynq 服务器配置，控制并发、重试等行为
//   - logger: *logging.Logger - 日志记录器实例，用于记录服务器日志
//   - hFuncs: HandlerFuncs - 任务处理函数映射表，定义任务类型到处理函数的映射
//
// 返回值：
//   - *Server - 初始化完成的任务队列服务器实例
//
// 处理流程：
//  1. 创建任务分发器（ServeMux）
//  2. 在调试模式下添加 panic 恢复中间件
//  3. 注册所有任务处理函数到分发器
//  4. 创建 asynq 服务器实例并连接到 Redis
//  5. 返回配置完成的 Server 实例
//
// 使用示例：
//
//	// 创建 Redis 客户端
//	redisClient := redis.NewClient(redisConfig)
//
//	// 配置 asynq 服务器
//	asynqCfg := asynq.Config{
//	    Concurrency: 10,
//	    Queues: map[string]int{"default": 1},
//	}
//
//	// 定义任务处理函数
//	handlers := HandlerFuncs{
//	    "email:send": sendEmailHandler,
//	}
//
//	// 创建服务器
//	server := pipeline.NewServer(redisClient, asynqCfg, logger, handlers)
//
//	// 启动服务器
//	if err := server.Start(); err != nil {
//	    log.Fatal(err)
//	}
//
// 注意事项：
//  1. 服务器创建后需要调用 Start 方法才能开始处理任务
//  2. 在生产环境中应该使用 defer server.Shutdown() 确保资源清理
//  3. 处理函数应该是幂等的，因为任务可能会被重试
//  4. 在 DEBUG_ENV_VAR 为 true 时会启用 panic 恢复中间件
func NewServer(cli *redis.Client, cfg RawConfig, logger *logging.Logger, hFuncs HandlerFuncs) *Server {
	// 创建任务分发器，负责根据任务类型路由到对应的处理函数
	mux := asynq.NewServeMux()

	// 在调试环境下添加 panic 恢复中间件
	// 该中间件会捕获任务处理过程中的 panic 并记录调用栈
	// 防止单个任务的异常导致整个服务器崩溃
	if base.DEBUG_ENV_VAR {
		mux.Use(RecoverWithStack(logger))
	}

	// 注册所有任务处理函数到分发器
	// 每个任务类型对应一个处理函数
	for pattern, hFunc := range hFuncs {
		mux.HandleFunc(pattern, hFunc)
		// 记录已加载的任务处理函数，便于监控和调试
		logger.Infof("load pipeline task handle func `%s`", pattern)
	}

	// 返回配置完成的服务器实例
	return &Server{
		logger: logger,                                   // 日志记录器，用于记录服务器运行状态
		srv:    asynq.NewServerFromRedisClient(cli, cfg), // 创建 asynq 服务器并连接到 Redis
		mux:    mux,                                      // 配置好的任务分发器
	}
}

// Start 启动任务队列服务器，开始从 Redis 队列中获取并处理任务。
// 该方法会启动后台 goroutine 来监听任务队列并分发任务给注册的处理函数。
//
// 返回值：
//   - error - 启动过程中发生的错误，nil 表示成功启动
//
// 工作流程：
//  1. 启动 asynq 服务器，开始监听 Redis 队列
//  2. 根据配置的并发数创建 worker goroutine
//  3. 从队列中获取任务并分发给对应的处理函数
//  4. 处理任务执行结果（成功、失败、重试）
//
// 使用示例：
//
//	server := NewServer(redisClient, cfg, logger, handlers)
//	if err := server.Start(); err != nil {
//	    logger.Errorf("Failed to start pipeline server: %v", err)
//	    return
//	}
//	defer server.Shutdown()
//
// 注意事项：
//  1. 该方法会阻塞当前 goroutine，直到服务器被关闭
//  2. 应该在独立的 goroutine 中调用，避免阻塞主程序
//  3. 启动失败可能是由于 Redis 连接问题或配置错误
//  4. 服务器启动后可以通过 Shutdown 方法优雅关闭
//  5. 所有错误都会通过 errors.WithStack 包装，便于调试
func (s *Server) Start() error {
	// 启动 asynq 服务器，开始处理队列中的任务
	// 该方法会启动 worker goroutine 并开始消费任务队列
	if err := s.srv.Start(s.mux); err != nil {
		// 使用 errors.WithStack 包装错误，添加堆栈跟踪信息
		// 便于在生产环境中定位问题
		return errors.WithStack(err)
	}
	// 成功启动，返回 nil
	return nil
}

// Shutdown 优雅关闭任务队列服务器。
// 该方法会停止接收新任务，等待正在处理的任务完成，然后释放所有资源。
//
// 关闭流程：
//  1. 停止从 Redis 队列获取新任务
//  2. 等待所有正在执行的任务完成（根据配置的超时时间）
//  3. 关闭 Redis 连接
//  4. 释放所有相关资源
//
// 使用示例：
//
//	// 启动服务器
//	go func() {
//	    if err := server.Start(); err != nil {
//	        logger.Errorf("Server error: %v", err)
//	    }
//	}()
//
//	// 处理程序退出信号
//	sigCh := make(chan os.Signal, 1)
//	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
//	<-sigCh
//
//	// 优雅关闭服务器
//	server.Shutdown()
//	logger.Info("Pipeline server shutdown completed")
//
// 注意事项：
//  1. 该方法会阻塞直到所有任务完成或超时
//  2. 应该在程序退出前调用，确保资源正确释放
//  3. 正在执行的任务会被允许完成，不会强制中断
//  4. 新任务会被拒绝，返回适当的错误
//  5. 关闭后服务器不能重新启动，需要创建新的实例
func (s *Server) Shutdown() {
	// 调用 asynq 服务器的 Shutdown 方法
	// 该方法会优雅关闭服务器，等待正在处理的任务完成
	s.srv.Shutdown()
}

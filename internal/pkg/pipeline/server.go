// Package queue 提供基于 asynq 的任务队列和调度器功能。
// 该包包含了任务队列客户端、服务器、调度器、配置管理和中间件等功能模块，
// 用于构建分布式任务处理系统。事件驱动型任务的投递与消费统一由本包承载，
// 与数据处理流水线（etl 包）解耦；分布式常驻处理器由外部运行时直接编排。

package pipeline

import (
	"context"

	"github.com/hibiken/asynq"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
)

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
// 它只负责事件驱动任务的消费，不管理分布式常驻处理器（Processor）。
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
//   - hFuncs: etl.HandlerFuncs - 任务处理函数映射表，定义任务类型到处理函数的映射
//   - mws: ...asynq.MiddlewareFunc - 附加的中间件函数（如 OTEL trace 中间件）
//
// 返回值：
//   - *Server - 初始化完成的任务队列服务器实例
//
// 处理流程：
//  1. 创建任务分发器（ServeMux）
//  2. 注册 panic 恢复中间件与附加中间件
//  3. 注册所有任务处理函数到分发器
//  4. 创建 asynq 服务器实例并连接到 Redis
//  5. 返回配置完成的 Server 实例
func NewServer(ctx context.Context, cli *redis.Client, cfg RawConfig, logger *logging.Logger, hFuncs etl.HandlerFuncs, mws ...asynq.MiddlewareFunc) *Server {
	// 创建任务分发器，负责根据任务类型路由到对应的处理函数
	mux := asynq.NewServeMux()

	// 错误边界中间件：捕获 panic + 记录错误堆栈
	// 始终启用，确保生产环境也能看到完整的错误信息
	mux.Use(RecoverWithStack(logger))

	// 注册额外的中间件（如 OTEL trace 中间件）
	for _, mw := range mws {
		mux.Use(mw)
	}

	// 注册所有任务处理函数到分发器
	// 每个任务类型对应一个处理函数；etl.HandlerFunc 消费 etl.Task 抽象，
	// 此处将 *asynq.Task（满足 etl.Task）适配回 asynq 原生签名
	for pattern, hFunc := range hFuncs {
		mux.HandleFunc(pattern, func(ctx context.Context, task *asynq.Task) error {
			return hFunc(ctx, task)
		})
		// 记录已加载的任务处理函数，便于监控和调试
		logger.Ctx(ctx).Infof("load pipeline task handle func `%s`", pattern)
	}

	srv := asynq.NewServerFromRedisClient(cli.Client, cfg)

	// 返回配置完成的服务器实例
	return &Server{
		logger: logger, // 日志记录器，用于记录服务器运行状态
		srv:    srv,    // 创建 asynq 服务器并连接到 Redis
		mux:    mux,    // 配置好的任务分发器
	}
}

// Start 启动任务队列服务器，开始从 Redis 队列中获取并处理任务。
// 该方法会阻塞当前 goroutine，持续处理任务直到服务器被关闭。
//
// 返回值：
//   - error - 启动过程中发生的错误，nil 表示成功启动并优雅关闭
//
// 工作流程：
//  1. 启动 asynq 服务器，开始监听 Redis 队列
//  2. 根据配置的并发数创建 worker goroutine
//  3. 从队列中获取任务并分发给对应的处理函数
//  4. 处理任务执行结果（成功、失败、重试）
//
// 注意事项：
//  1. 该方法会阻塞当前 goroutine，直到服务器被关闭
//  2. 应该在独立的 goroutine 中调用，避免阻塞主程序
//  3. 启动失败可能是由于 Redis 连接问题或配置错误
//  4. 服务器启动后可以通过 Shutdown 方法优雅关闭
//  5. 所有错误都会通过 errors.WithStack 包装，便于调试
func (s *Server) Start(ctx context.Context) error {
	// 启动 asynq 服务器，开始处理队列中的任务
	// Run 会阻塞当前 goroutine，持续处理任务直到收到退出信号或 Shutdown 被调用。
	// ctx 参数仅保持调用签名一致；asynq 内部通过信号处理实现优雅退出。
	if err := s.srv.Run(s.mux); err != nil {
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

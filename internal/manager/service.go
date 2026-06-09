// Package manager 提供 API 管理层的核心功能，包括：
//   - HTTP 服务的启动与生命周期管理（本文件）
//   - 配置管理（config.go）
//   - JWT 认证与令牌管理（authenticate.go）
//   - RBAC 授权策略管理（authorize.go）
//   - 安全中间件（secure.go）
//   - 数据库、日志、国际化等基础设施初始化
//
// 本文件（service.go）定义了基于 Uber FX 依赖注入框架的 HTTP 服务启动函数，
// 负责在应用启动时异步启动 HTTP 服务器，在应用停止时优雅关闭服务器。
package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/depend"
)

// HttpFxIn 定义了 HTTP 服务启动所需的依赖注入参数集合。
//
// 该结构体使用 fx.In 标记，表示 Uber FX 框架将自动从依赖容器中
// 解析并注入所有字段。通过聚合 Config、Lifecycle、Gin Engine 和
// API 日志记录器等依赖，为 HTTP 服务的启动与生命周期管理提供完整支持。
//
// 依赖说明：
//   - Cfg: API 管理层配置，包含服务监听地址和端口等参数
//   - Lc: FX 生命周期管理器，用于注册服务启动和停止的钩子函数
//   - Engine: Gin 路由引擎，作为 HTTP 请求的处理器
//   - APILoggerFxIn: API 专用日志记录器，用于记录服务运行状态
type HttpFxIn struct {
	fx.In
	Cfg    *Config
	Lc     fx.Lifecycle
	Engine *gin.Engine
	depend.APILoggerFxIn
}

// RunHttpServiceFx 基于 Uber FX 框架启动 HTTP 服务，并注册生命周期钩子。
//
// 该函数执行以下操作：
//  1. 根据 Config 中的 Host 和 Port 构造监听地址，创建 http.Server 实例
//  2. 通过 fx.Lifecycle 注册 OnStart 钩子，在应用启动时以协程方式异步启动 HTTP 服务器，
//     避免阻塞 FX 应用的初始化流程；若启动失败（非 http.ErrServerClosed 错误），
//     则通过日志记录器以 Fatal 级别终止进程
//  3. 通过 fx.Lifecycle 注册 OnStop 钩子，在应用停止时优雅关闭 HTTP 服务器，
//     使用 context.Context 控制关闭超时，确保正在处理的请求能够完成
//
// 参数：
//   - in: HTTP 服务所需的全部依赖注入参数，包含配置、生命周期、路由引擎和日志记录器
//
// 注意事项：
//   - HTTP 服务器在 OnStart 中以协程方式启动，因此不会阻塞 FX 生命周期启动流程
//   - http.ErrServerClosed 是调用 Shutdown() 后的正常返回，不视为错误
//   - OnStop 中的 Shutdown 会等待活跃连接处理完毕或上下文取消
func RunHttpServiceFx(in HttpFxIn) {
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", in.Cfg.Host, in.Cfg.Port),
		Handler: in.Engine,
	}
	in.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			go func() {
				in.APILogger.Info("web server listening on")
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					in.APILogger.Fatal("failed to start http server", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			in.APILogger.Info("web server forced to shutdown")
			return srv.Shutdown(ctx)
		},
	})
}

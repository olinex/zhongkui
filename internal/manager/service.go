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

	"github.com/samber/do/v2"
	"go.uber.org/zap"
	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// RunHttpService 启动 HTTP 服务并在 context 取消时优雅关闭。
// 服务在 goroutine 中监听并在启动失败或收到关闭信号时返回错误。
func RunHttpService(ctx context.Context, i do.Injector) error {
	viper := provider.MustInvokeViper(i)
	engine := provider.MustInvokeGin(i)
	apiLogger := provider.MustInvokeApiLogger(i)
	cfg := config.MustExtractConfig(viper, validator.Validate)
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler: engine,
	}

	// 在 goroutine 中启动 HTTP 服务
	errCh := make(chan error, 1)
	go func() {
		apiLogger.Ctx(ctx).Info("web server listening on")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- errors.WithStack(err)
		}
	}()

	// 等待 context 取消或错误
	select {
	case <-ctx.Done():
		// 收到信号，开始优雅关闭
		apiLogger.Ctx(ctx).Info("web server received shutdown signal")
		return errors.WithStack(srv.Shutdown(ctx))
	case err := <-errCh:
		// 服务启动失败
		apiLogger.Ctx(ctx).Fatal("failed to start http server", zap.Error(err))
		return errors.WithStack(err)
	}
}

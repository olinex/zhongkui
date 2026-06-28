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
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/provider"
)

func RunHttpService(ctx context.Context, i do.Injector) error {
	viper := provider.MustInvokeViper(i)
	engine := provider.MustInvokeGin(i)
	apiLogger := provider.MustInvokeApiLogger(i)
	cfg, err := ExtractConfig(viper)
	if err != nil {
		return errors.WithStack(err)
	}
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler: engine,
	}
	apiLogger.Ctx(ctx).Info("web server listening on")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		apiLogger.Ctx(ctx).Fatal("failed to start http server", zap.Error(err))
	} else if ctx != nil {
		apiLogger.Ctx(ctx).Warn("http server received error", zap.Error(err))
	}
	apiLogger.Ctx(ctx).Info("web server forced to shutdown")
	return srv.Shutdown(ctx)
}

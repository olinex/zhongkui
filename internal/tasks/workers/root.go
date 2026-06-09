package workers

import (
	"context"

	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/depend"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

const (
	// CONTEXT_FX_IN_NAME 上下文中的 FX 依赖注入上下文的键名
	CONTEXT_FX_IN_NAME = "ctx_fx_in"
)

// ContextFxIn FX 依赖注入的上下文结构。
// 包含所有需要注入的依赖项和生命周期管理。
type ContextFxIn struct {
	fx.In
	depend.ManageEtcdFxIn      // etcd 管理接口
	depend.CaptureS3ClientIn   // S3 客户端接口
	depend.PipelineLoggerFxIn  // 日志接口
	depend.PipelineRedisFxIn   // Redis 接口
	depend.RawDatabaseFxIn     // 原始数据库接口
	Lc  fx.Lifecycle           // 生命周期管理
	Ctx context.Context        // 上下文
}

// TaskContext 创建任务上下文的函数。
// 返回一个函数，用于构建包含 FX 依赖注入的上下文。
func TaskContext(in ContextFxIn) pipeline.ContextFunc {
	return func() context.Context {
		return context.WithValue(in.Ctx, CONTEXT_FX_IN_NAME, &in)
	}
}

// GetFxIn 从上下文中获取 FX 依赖注入的上下文。
func GetFxIn(ctx context.Context) *ContextFxIn {
	return ctx.Value(CONTEXT_FX_IN_NAME).(*ContextFxIn)
}

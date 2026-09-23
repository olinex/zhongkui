package provider

import (
	// stdlib
	"context"

	// third-party
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// newAiProvider 装配 AI 助手运行时（ai.Runtime）：以数据库模型注册表
// （llm_models）、档位绑定（llm_bindings）与参数覆盖（agent_params）作为 LLM 接入配置的
// 唯一来源，装配 core 库句柄、etcd 客户端（变更监听）与日志记录器后构造运行时。
// 首次装配失败（如 embedding 维度与库列不一致、绑定模型端点信息不完整）返回错误，
// 由启动路径经 MustInvokeAi panic 中止启动；模型注册表为空时装配为未启用状态而不
// 报错，服务可正常启动等待前端注册模型。
// newAiProvider assembles the AI assistant runtime (ai.Runtime): it takes the
// database model registry (llm_models), the slot bindings (llm_bindings) and the parameter
// overrides (agent_params) as the single source of the LLM access config, resolving the core
// DB handle, the etcd client (change watching) and the logger before constructing the runtime.
// A failed first assembly (e.g. an embedding dimension mismatch with the DB column or an
// incomplete bound-model endpoint) returns an error so the startup path aborts through
// MustInvokeAi panicking; an empty model registry assembles into the disabled state
// without error, letting the service start and wait for models to be registered.
func newAiProvider(ctx context.Context) do.Provider[*ai.Runtime] {
	return func(i do.Injector) (*ai.Runtime, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return ai.NewRuntime(ctx, db, client, logger)
		}
	}
}

// InjectAi 向依赖注入器注册 AI 助手运行时 provider。
// InjectAi registers the AI assistant runtime provider into the injector.
func InjectAi(ctx context.Context, i do.Injector) {
	do.Provide(i, newAiProvider(ctx))
}

// InvokeAi 从依赖注入器解析 AI 助手运行时，错误在调用方处理。
// InvokeAi resolves the AI assistant runtime from the injector; errors are handled by
// the caller.
func InvokeAi(ctx context.Context, i do.Injector) (*ai.Runtime, error) {
	return do.Invoke[*ai.Runtime](i)
}

// MustInvokeAi 从依赖注入器解析 AI 助手运行时；失败时 panic（仅用于启动关键依赖）。
// MustInvokeAi resolves the AI assistant runtime from the injector; it panics on
// failure (used only for startup-critical dependencies).
func MustInvokeAi(ctx context.Context, i do.Injector) *ai.Runtime {
	return do.MustInvoke[*ai.Runtime](i)
}

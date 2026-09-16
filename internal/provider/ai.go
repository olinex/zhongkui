package provider

import (
	// stdlib

	// third-party
	"context"

	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"

	"zhongkui.com/zhongkui/internal/pkg/llm"
)

// DescribeAiConfig 返回 AI（[llm]）配置的结构化描述信息（供 describe config 渲染）。
// DescribeAiConfig returns the structured description of the AI ([llm]) config (rendered by describe config).
func DescribeAiConfig() config.ConfigDescriptions {
	return llm.DescribeConfig()
}

// newAiProvider 装配 AI 助手（RAG + Agent）运行所需的全部组件：提取 [llm] 配置（未配置
// [llm] 段时按填充默认值的空配置处理，AI 自然关闭）；AI 未启用时仅返回携带配置的
// Components（其余组件为 nil，启动不失败）；启用后依次构造客户端、会话存储、知识库、
// 检索器与 Agent，任一装配失败返回错误，不允许以残缺能力运行。
// newAiProvider assembles all the components the AI assistant (RAG + Agent) needs at
// runtime: it extracts the [llm] config (a defaulted empty config when the [llm] section
// is absent, so AI is naturally off); when AI is disabled it returns a Components
// carrying only the config (the other components are nil and startup does not fail);
// when enabled it constructs the clients, session store, knowledge, retriever and
// Agent in order, returning an error on any failure so the capability never runs
// crippled.
func newAiProvider(ctx context.Context) do.Provider[*ai.Components] {
	return func(i do.Injector) (*ai.Components, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		aiCfg, err := llm.ExtractConfigOrEmpty(viper, validator.Validate)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		comps := &ai.Components{Config: aiCfg}
		if !aiCfg.Enabled() {
			return comps, nil // 未启用：仅携带配置，其余组件为 nil，启动不失败
		}
		coreDB, err := InvokeCoreDatabase(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 启动时强校验 knowledge_chunks.embedding 列维度与配置一致：不一致即中止启动
		// （经 MustInvokeAi panic），用户需自行执行 migrate 子命令重建列，而非在此自动改库。
		// Strictly verify at startup that the knowledge_chunks.embedding column dimension matches
		// the config: a mismatch aborts startup (via MustInvokeAi panicking) and the user is
		// expected to reconcile the column themselves through the migrate subcommand, rather than
		// auto-altering the schema here.
		if err := llm.CheckEmbeddingDimension(ctx, coreDB, aiCfg.Embedding.Dimensions); err != nil {
			return nil, errors.WithStack(err)
		}
		clients, err := llm.NewClients(ctx, aiCfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sessions, err := ai.NewSessionStore(coreDB)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		knowledge, err := ai.NewKnowledge(coreDB, clients.Embed, clients.Chat, aiCfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		retriever, err := ai.NewRetriever(coreDB, clients.Embed, clients.Chat, aiCfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		agent, err := ai.NewAgent(clients.Chat, retriever, sessions, aiCfg)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		comps.Agent, comps.Knowledge, comps.Sessions = agent, knowledge, sessions
		return comps, nil
	}
}

// InjectAi 向依赖注入器注册 AI 助手组件 provider。
// InjectAi registers the AI assistant components provider into the injector.
func InjectAi(ctx context.Context, i do.Injector) {
	do.Provide(i, newAiProvider(ctx))
}

// InvokeAi 从依赖注入器解析 AI 助手组件，错误在调用方处理。
// InvokeAi resolves the AI assistant components from the injector; errors are handled by the caller.
func InvokeAi(ctx context.Context, i do.Injector) (*ai.Components, error) {
	return do.Invoke[*ai.Components](i)
}

// MustInvokeAi 从依赖注入器解析 AI 助手组件；失败时 panic（仅用于启动关键依赖）。
// MustInvokeAi resolves the AI assistant components from the injector; it panics on
// failure (used only for startup-critical dependencies).
func MustInvokeAi(ctx context.Context, i do.Injector) *ai.Components {
	return do.MustInvoke[*ai.Components](i)
}

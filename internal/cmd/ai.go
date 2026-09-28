package cmd

import (
	"context"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
)

// migrateAiEmbeddingCmd 按运维经 --dimensions 显式指定的目标维度重建 core 库的
// knowledge_chunks.embedding 与 agent_memories.embedding 两列（逐列删 HNSW 索引、清空旧向量、
// ALTER TYPE、重建索引）。清空旧向量后需重新索引知识库文档，记忆向量需重新积累。独立于
// migrate up：索引维度是运维独占的 schema 属性，换用不同维度的 embedding 模型后由运维显式
// 执行本命令调整。
var migrateAiEmbeddingCmd = &cobra.Command{
	Use:   "ai-embedding",
	Short: "cmd.migrate.ai-embedding.short",
	Long:  "cmd.migrate.ai-embedding.long",
	Run:   runReconcileEmbeddingCmd,
}

// runReconcileEmbeddingCmd 执行 embedding 列维度重建：经 viper 读取 --dimensions 的目标维度
// （命令行参数已在启动期绑定进 viper，无需再访问 cmd.Flags），交由 reconcileCoreAISchema
// 完成冲突检查与 DDL 重建。
func runReconcileEmbeddingCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	gormDB := provider.MustInvokeDatabase(i, base.CoreDbName)
	v := provider.MustInvokeViper(i)

	pterm.DefaultSection.Println(translate.T("cli.migrate.ai-embedding-section", map[string]any{"DB": base.CoreDbName}))
	if err := reconcileCoreAISchema(ctx, gormDB, v.GetInt("dimensions")); err != nil {
		pterm.Error.Println(translate.T("cli.migrate.ai-embedding-failed", map[string]any{"DB": base.CoreDbName, "Err": err}))
		return
	}
}

// reconcileCoreAISchema 按运维显式指定的目标维度重建 knowledge_chunks.embedding 与
// agent_memories.embedding 两列：目标维度只来自 --dimensions（不再是模型注册表/配置声明的维度），
// 必须先校验为正整数；若 embedding 槽位已绑定模型且其维度与目标维度不同（维度未知也视为不同），
// 则拒绝并在错误中提示先解绑（拒绝规则保证了真正重建时 embedding 槽位必然未绑定，故无需广播
// 运行时失效）；通过校验后执行两列维度的实际变更（ALTER TYPE、清空旧向量、重建 HNSW 索引），
// 完成后需重新索引知识库文档、记忆向量需重新积累。
func reconcileCoreAISchema(ctx context.Context, gormDB *database.DB, target int) error {
	if target <= 0 {
		return errors.New("the --dimensions flag is required and must be a positive integer")
	}
	store, err := ai.NewStore(gormDB)
	if err != nil {
		return errors.Newf("failed to read ai model registry: %v", err)
	}
	name, dimensions, bound, err := store.EmbeddingDimensions(ctx)
	if err != nil {
		return errors.Newf("failed to read ai embedding dimension: %v", err)
	}
	if bound && dimensions != target {
		return errors.Newf(
			"the embedding slot is bound to model %q with %d dimensions; unbind it before resizing the index to %d",
			name, dimensions, target,
		)
	}
	reconciled, err := llm.ReconcileEmbeddingColumn(ctx, gormDB, target)
	if err != nil {
		return errors.Newf("failed to reconcile ai embedding column: %v", err)
	}
	if reconciled {
		pterm.Success.Println(translate.T("cli.migrate.ai-embedding-reconciled", map[string]any{"Dimensions": target}))
	} else {
		pterm.Success.Println(translate.T("cli.migrate.ai-embedding-in-sync", nil))
	}
	return nil
}

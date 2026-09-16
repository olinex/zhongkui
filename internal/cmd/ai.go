package cmd

import (
	"context"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

// migrateAiEmbeddingCmd 将 core 库的 knowledge_chunks.embedding 列维度校准为 AI 配置
// 指定的维度。独立于 migrate up，由用户在更换 embedding 模型维度后手动触发。
// migrateAiEmbeddingCmd reconciles the knowledge_chunks.embedding column dimension on the core
// database to the AI-configured dimension. It is independent of migrate up and is triggered
// manually by the user after switching the embedding model dimension.
var migrateAiEmbeddingCmd = &cobra.Command{
	Use:   "ai-embedding",
	Short: "cmd.migrate.ai-embedding.short",
	Long:  "cmd.migrate.ai-embedding.long",
	Run:   runReconcileEmbeddingCmd,
}

// runReconcileEmbeddingCmd 执行 embedding 列维度校准：将 core 库的
// knowledge_chunks.embedding 列重建为 AI 配置指定的维度（删 HNSW 索引、清旧向量、
// ALTER TYPE、重建索引）。该命令独立于 migrate up，由用户在更换 embedding 模型维度后
// 手动触发（migrate ai-embedding）。
// runReconcileEmbeddingCmd rebuilds the knowledge_chunks.embedding column to the AI-configured
// dimension on the core database (drop HNSW index, clear stale vectors, ALTER TYPE, rebuild the
// index). It is independent of migrate up and is triggered manually by the user after switching
// the embedding model dimension.
func runReconcileEmbeddingCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	v := provider.MustInvokeViper(i)
	gormDB := provider.MustInvokeDatabase(i, base.CoreDbName)

	pterm.DefaultSection.Println(translate.T("cli.migrate.ai-embedding-section", map[string]any{"DB": base.CoreDbName}))
	if err := reconcileCoreAISchema(ctx, v, gormDB); err != nil {
		pterm.Error.Println(translate.T("cli.migrate.ai-embedding-failed", map[string]any{"DB": base.CoreDbName, "Err": err}))
		return
	}
}

// reconcileCoreAISchema 将 knowledge_chunks.embedding 列维度校准为 AI 配置指定的维度。
// AI 维度以配置为唯一事实来源，维度的实际变更（ALTER TYPE、删除旧向量、重建 HNSW 索引）
// 由用户在 ai reconcile-embedding 命令中显式触发，而非服务启动或 migrate 时自动改动。
// reconcileCoreAISchema reconciles the knowledge_chunks.embedding column dimension to the
// AI-configured dimension. The dimension is driven by the config as the single source of truth,
// and the actual schema change (ALTER TYPE, clearing stale vectors, rebuilding the HNSW index) is
// triggered explicitly by the user via the ai reconcile-embedding command, rather than being
// altered automatically at server startup or during migrate.
func reconcileCoreAISchema(ctx context.Context, v *viper.Viper, gormDB *database.DB) error {
	aiCfg, err := llm.ExtractConfigOrEmpty(v, validator.Validate)
	if err != nil {
		return errors.Newf("failed to extract ai config: %v", err)
	}
	reconciled, err := llm.ReconcileEmbeddingColumn(ctx, gormDB, aiCfg.Embedding.Dimensions)
	if err != nil {
		return errors.Newf("failed to reconcile ai embedding column: %v", err)
	}
	if reconciled {
		pterm.Success.Println(translate.T("cli.ai.embedding-reconciled", map[string]any{"Dimensions": aiCfg.Embedding.Dimensions}))
	} else {
		pterm.Success.Println(translate.T("cli.ai.embedding-in-sync", nil))
	}
	return nil
}

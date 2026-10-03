package cmd

import (
	// 标准库
	"context"
	"os"
	"strings"

	// 第三方
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	// 项目内部
	ai "zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/docs"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/docconv"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
	"zhongkui.com/zhongkui/internal/provider"
)

// syncCmd 是同步与向量对齐命令组：同步注释生成文档到知识库（RAG），并以当前 embedding 模型
// 重嵌入内容、对齐向量列维度。
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "cmd.sync.short",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// syncDocsCmd 从源码注释同步生成文档到指定命名空间的 RAG：叙述页（包注释自动生成）落
// format=generated；按内容 hash 差异计划幂等同步，只读写该分区、不触碰人工文档。走常规工具箱以
// 取得数据库与 embedding；不复用 create docs 的离线注入器。
var syncDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "cmd.sync.docs.short",
	Long:  "cmd.sync.docs.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		namespace := strings.TrimSpace(v.GetString("namespace"))
		if namespace == "" {
			pterm.Error.Println(translate.T("cli.sync.docs.no-namespace", nil))
			return
		}
		rt := provider.MustInvokeAi(ctx, i)
		if !rt.Enabled() {
			pterm.Error.Println(translate.T("cli.sync.docs.disabled", nil))
			return
		}
		cwd, err := os.Getwd()
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.docs.failed", map[string]any{"Err": err}))
			return
		}
		// 叙述页分区：由注册表中的自动生成条目渲染包注释为叙述档文档。
		narratives, err := docs.GeneratedNarrativeDocuments(cwd)
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.docs.failed", map[string]any{"Err": err}))
			return
		}
		narrativeDocs := make([]ai.GeneratedKnowledgeDoc, 0, len(narratives))
		for _, n := range narratives {
			narrativeDocs = append(narrativeDocs, ai.GeneratedKnowledgeDoc{Key: n.Key, Title: n.Title, Content: n.Content})
		}
		if err := rt.Current().Knowledge.SyncGeneratedDocs(ctx, namespace, docconv.FormatGenerated, narrativeDocs); err != nil {
			pterm.Error.Println(translate.T("cli.sync.docs.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.docs.done", map[string]any{
			"Namespace":  namespace,
			"Narratives": len(narrativeDocs),
		}))
	},
}

// syncEmbeddingsCmd 以当前 embedding 模型为准，重算指定命名空间下的向量表示（知识库文档经
// Reindex 重建分块与向量，长期记忆重算 embedding 列）。用于更换 embedding 模型（维度或语义
// 变化）后让既有内容与当前模型对齐；不改正文、不改全文检索列，不触碰其它命名空间。走常规
// 工具箱以取得数据库与 embedding。列维度变更请先执行 sync embedding-dimensions。
var syncEmbeddingsCmd = &cobra.Command{
	Use:   "embeddings",
	Short: "cmd.sync.embeddings.short",
	Long:  "cmd.sync.embeddings.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)
		namespace := strings.TrimSpace(v.GetString("namespace"))
		if namespace == "" {
			pterm.Error.Println(translate.T("cli.sync.embeddings.no-namespace", nil))
			return
		}
		rt := provider.MustInvokeAi(ctx, i)
		if !rt.Enabled() {
			pterm.Error.Println(translate.T("cli.sync.embeddings.disabled", nil))
			return
		}
		comps := rt.Current()
		docs, err := comps.Knowledge.ReindexNamespace(ctx, namespace)
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.embeddings.failed", map[string]any{"Err": err}))
			return
		}
		memories, err := comps.Memory.ReindexEmbeddings(ctx, namespace)
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.embeddings.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.embeddings.done", map[string]any{
			"Namespace": namespace,
			"Documents": docs,
			"Memories":  memories,
		}))
	},
}

// syncEmbeddingDimensionsCmd 按运维经 --dimensions 显式指定的目标维度重建 core 库的
// knowledge_chunks.embedding 与 agent_memories.embedding 两列（逐列删 HNSW 索引、清空旧向量、
// ALTER TYPE、重建索引）。清空旧向量后需重新索引知识库文档，记忆向量需重新积累。它与
// sync embeddings 是同一命令组内的并列两条：本命令只改 schema（列维度），sync embeddings 只改
// 内容（重嵌入向量），二者互不替代；维度变更须先执行本命令，再执行 sync embeddings 重建内容。
var syncEmbeddingDimensionsCmd = &cobra.Command{
	Use:   "embedding-dimensions",
	Short: "cmd.sync.embedding-dimensions.short",
	Long:  "cmd.sync.embedding-dimensions.long",
	Run:   runReconcileEmbeddingCmd,
}

// runReconcileEmbeddingCmd 执行 embedding 列维度重建：经 viper 读取 --dimensions 的目标维度
// （命令行参数已在启动期绑定进 viper，无需再访问 cmd.Flags），交由 reconcileCoreAISchema
// 完成冲突检查与 DDL 重建。
func runReconcileEmbeddingCmd(cmd *cobra.Command, args []string) {
	ctx, i := cmdToolBox(cmd)
	gormDB := provider.MustInvokeDatabase(i, base.CoreDbName)
	v := provider.MustInvokeViper(i)

	pterm.DefaultSection.Println(translate.T("cli.sync.embedding-dimensions-section", map[string]any{"DB": base.CoreDbName}))
	if err := reconcileCoreAISchema(ctx, gormDB, v.GetInt("dimensions")); err != nil {
		pterm.Error.Println(translate.T("cli.sync.embedding-dimensions-failed", map[string]any{"DB": base.CoreDbName, "Err": err}))
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
	reconciled, err := ai.ReconcileEmbeddingColumn(ctx, gormDB, target)
	if err != nil {
		return errors.Newf("failed to reconcile ai embedding column: %v", err)
	}
	if reconciled {
		pterm.Success.Println(translate.T("cli.sync.embedding-dimensions-reconciled", map[string]any{"Dimensions": target}))
	} else {
		pterm.Success.Println(translate.T("cli.sync.embedding-dimensions-in-sync", nil))
	}
	return nil
}

func init() {
	syncDocsCmd.Flags().String("namespace", "", "flag.sync.namespace")
	syncEmbeddingsCmd.Flags().String("namespace", "", "flag.sync.namespace")
	syncEmbeddingDimensionsCmd.Flags().Int("dimensions", 0, "flag.sync.embedding-dimensions.dimensions")
}

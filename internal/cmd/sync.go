package cmd

import (
	"zhongkuisec.com/zhongkui/internal/ai/llminfra"
	// 标准库
	"context"
	"os"
	"strings"

	// 第三方
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/capability"
	"zhongkuisec.com/zhongkui/internal/ai/knowledge"
	"zhongkuisec.com/zhongkui/internal/ai/llmconfig"
	"zhongkuisec.com/zhongkui/internal/docs"
	"zhongkuisec.com/zhongkui/internal/manager"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/provider"
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

// syncDocsCmd 从源码注释同步生成文档到 RAG：叙述页（包注释自动生成）落 format=generated，
// 幂等同步到内置系统命名空间（base.SystemNamespace）：按内容 hash 差异计划新增、更新正文变化的
// 条目、删除已下线条目，正文未变的条目零写入，只读写该分区、不触碰人工文档。叙述页是代码声明的
// 公开共享数据，对所有命名空间的检索可见，与 sync apis 的语义一致，故无需运维指定命名空间。
// 走常规工具箱以取得数据库与 embedding；不复用 create docs 的离线注入器。
var syncDocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "cmd.sync.docs.short",
	Long:  "cmd.sync.docs.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
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
		narrativeDocs := make([]knowledge.GeneratedKnowledgeDoc, 0, len(narratives))
		for _, n := range narratives {
			narrativeDocs = append(narrativeDocs, knowledge.GeneratedKnowledgeDoc{Key: n.Key, Title: n.Title, Content: n.Content})
		}
		model := embeddingModelName(rt)
		if err := rt.Current().Knowledge.SyncGeneratedDocs(ctx, base.SystemNamespace, docconv.FormatGenerated, narrativeDocs, newSyncProgress(base.SystemNamespace, model)); err != nil {
			pterm.Error.Println(translate.T("cli.sync.docs.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.docs.done", map[string]any{
			"Namespace":  base.SystemNamespace,
			"Narratives": len(narrativeDocs),
		}))
	},
}

// syncApisCmd 把注册期声明的原生接口目录（单行用途 + 入参/出参元信息）渲染为知识文档，
// 幂等同步到内置系统命名空间（base.SystemNamespace）的 native_api 分区：新增、更新正文变化的
// 条目、删除已下线或已退出 RAG 的条目，正文未变的条目零写入。接口目录是代码声明的公开共享
// 数据，对所有命名空间的检索可见，故无需运维指定命名空间。目录经离线装配取得（无需 DB /
// Redis / Etcd），但写库与向量化仍走常规工具箱以取得数据库与 embedding；只读写 native_api
// 分区，不触碰其余分区。
var syncApisCmd = &cobra.Command{
	Use:   "apis",
	Short: "cmd.sync.apis.short",
	Long:  "cmd.sync.apis.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		rt := provider.MustInvokeAi(ctx, i)
		if !rt.Enabled() {
			pterm.Error.Println(translate.T("cli.sync.apis.disabled", nil))
			return
		}
		// 离线装配接口目录，仅取纳入 RAG 的接口渲染为生成文档；标题即稳定去重键。
		catalog := manager.CollectApiKnowledgeDocs()
		apiDocs := make([]knowledge.GeneratedKnowledgeDoc, 0, len(catalog))
		for _, doc := range catalog {
			apiDocs = append(apiDocs, knowledge.GeneratedKnowledgeDoc{Key: doc.Title, Title: doc.Title, Content: doc.Content})
		}
		model := embeddingModelName(rt)
		if err := rt.Current().Knowledge.SyncGeneratedDocs(ctx, base.SystemNamespace, docconv.FormatNativeApi, apiDocs, newSyncProgress(base.SystemNamespace, model)); err != nil {
			pterm.Error.Println(translate.T("cli.sync.apis.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.apis.done", map[string]any{
			"Namespace": base.SystemNamespace,
			"Apis":      len(apiDocs),
		}))
	},
}

// syncSchemaCmd 从模型声明与迁移脚本同步数据表知识到 RAG：每张已迁移表一篇，落 format=schema，
// 幂等同步到内置系统命名空间（base.SystemNamespace）。标题为稳定去重键；正文与数据表参考页同源，
// 只读写 schema 分区，不触碰人工文档、接口、能力与叙述页分区。
var syncSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "cmd.sync.schema.short",
	Long:  "cmd.sync.schema.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		rt := provider.MustInvokeAi(ctx, i)
		if !rt.Enabled() {
			pterm.Error.Println(translate.T("cli.sync.schema.disabled", nil))
			return
		}
		cwd, err := os.Getwd()
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.schema.failed", map[string]any{"Err": err}))
			return
		}
		generated, err := docs.SchemaKnowledgeDocuments(cwd)
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.schema.failed", map[string]any{"Err": err}))
			return
		}
		tableDocs := make([]knowledge.GeneratedKnowledgeDoc, 0, len(generated))
		for _, doc := range generated {
			tableDocs = append(tableDocs, knowledge.GeneratedKnowledgeDoc{Key: doc.Key, Title: doc.Title, Content: doc.Content})
		}
		model := embeddingModelName(rt)
		if err := rt.Current().Knowledge.SyncGeneratedDocs(ctx, base.SystemNamespace, docconv.FormatSchema, tableDocs, newSyncProgress(base.SystemNamespace, model)); err != nil {
			pterm.Error.Println(translate.T("cli.sync.schema.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.schema.done", map[string]any{
			"Namespace": base.SystemNamespace,
			"Tables":    len(tableDocs),
		}))
	},
}

// syncCapabilitiesCmd 从视图层能力声明同步能力知识到 RAG：一篇整体能力地图 + 每条能力一篇明细，
// 落 format=capability，幂等同步到内置系统命名空间（base.SystemNamespace）。标题为稳定去重键；
// 只读写 capability 分区，不触碰其余分区。能力由视图层随接口声明提供，属于代码声明的公开共享
// 数据，故无需运维指定命名空间。
var syncCapabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "cmd.sync.capabilities.short",
	Long:  "cmd.sync.capabilities.long",
	Run: func(cmd *cobra.Command, args []string) {
		ctx, i := cmdToolBox(cmd)
		rt := provider.MustInvokeAi(ctx, i)
		if !rt.Enabled() {
			pterm.Error.Println(translate.T("cli.sync.capabilities.disabled", nil))
			return
		}
		generated := capability.KnowledgeDocs(manager.CollectCapabilities())
		capDocs := make([]knowledge.GeneratedKnowledgeDoc, 0, len(generated))
		for _, doc := range generated {
			capDocs = append(capDocs, knowledge.GeneratedKnowledgeDoc{Key: doc.Title, Title: doc.Title, Content: doc.Content})
		}
		model := embeddingModelName(rt)
		if err := rt.Current().Knowledge.SyncGeneratedDocs(ctx, base.SystemNamespace, docconv.FormatCapability, capDocs, newSyncProgress(base.SystemNamespace, model)); err != nil {
			pterm.Error.Println(translate.T("cli.sync.capabilities.failed", map[string]any{"Err": err}))
			return
		}
		pterm.Success.Println(translate.T("cli.sync.capabilities.done", map[string]any{
			"Namespace":    base.SystemNamespace,
			"Capabilities": len(capDocs),
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
		model := embeddingModelName(rt)
		comps := rt.Current()
		docs, err := comps.Knowledge.ReindexNamespace(ctx, namespace, newSyncProgress(namespace, model))
		if err != nil {
			pterm.Error.Println(translate.T("cli.sync.embeddings.failed", map[string]any{"Err": err}))
			return
		}
		memories, err := comps.Memory.ReindexEmbeddings(ctx, namespace, newSyncProgress(namespace, model))
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
		return errors.New(translate.T("cli.sync.embedding-dimensions-flag-required", nil))
	}
	store, err := llmconfig.NewStore(gormDB)
	if err != nil {
		return errors.Newf("failed to read ai model registry: %v", err)
	}
	name, dimensions, bound, err := store.EmbeddingDimensions(ctx)
	if err != nil {
		return errors.Newf("failed to read ai embedding dimension: %v", err)
	}
	if bound && dimensions != target {
		return errors.New(translate.T("cli.sync.embedding-dimensions-slot-bound", map[string]any{
			"Name":       name,
			"Dimensions": dimensions,
			"Target":     target,
		}))
	}
	reconciled, err := llminfra.ReconcileEmbeddingColumn(ctx, gormDB, target)
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
	syncEmbeddingsCmd.Flags().String("namespace", "", "flag.sync.namespace")
	syncEmbeddingDimensionsCmd.Flags().Int("dimensions", 0, "flag.sync.embedding-dimensions.dimensions")
}

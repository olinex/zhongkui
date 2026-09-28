package llm

import (
	// stdlib
	"context"
	"fmt"
	"strconv"
	"strings"

	// third-party
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// 向量检索依赖的 pgvector 表/列/索引元数据，与 internal/migrations/core/001_init.sql 对齐。
// 知识库列常量供知识库语义函数复用；记忆列与知识库列同源，须一并校准。
// pgvector table/column/index metadata the vector retrieval relies on, aligned with
// internal/migrations/core/001_init.sql. The knowledge-column constants are reused by the
// knowledge-specific helpers; the memory column shares the same dimension and is reconciled
// together with the knowledge column.
const (
	chunkEmbeddingTable  = "knowledge_chunks"
	chunkEmbeddingColumn = "embedding"
	chunkEmbeddingIndex  = "idx_knowledge_chunks_embedding"

	memoryEmbeddingTable  = "agent_memories"
	memoryEmbeddingColumn = "embedding"
	memoryEmbeddingIndex  = "idx_agent_memories_embedding"
)

// vectorColumn 描述一列受维度治理的 pgvector 列及其 HNSW 索引。
// vectorColumn describes a pgvector column and its HNSW index under dimension governance.
type vectorColumn struct{ table, column, index string }

// vectorColumns 是全部受维度治理的向量列清单：知识库与长期记忆两列同源，须一并校准。
// vectorColumns is the full list of vector columns under dimension governance: the knowledge and
// long-term memory columns share the same dimension and are reconciled together.
var vectorColumns = []vectorColumn{
	{table: chunkEmbeddingTable, column: chunkEmbeddingColumn, index: chunkEmbeddingIndex},
	{table: memoryEmbeddingTable, column: memoryEmbeddingColumn, index: memoryEmbeddingIndex},
}

// ReconcileEmbeddingColumn 按 vectorColumns 清单遍历，将每列（knowledge_chunks.embedding
// 与 agent_memories.embedding）的维度校准为配置指定的维度，使「配置为唯一事实来源」：更换
// embedding 模型（通常伴随维度变化）时，用户只需改 ai.embedding.dimensions，启动后数据库列
// 会自动跟随，无需手工改迁移或重建索引。返回的 reconciled 表示「任一列被改动过」。
// ReconcileEmbeddingColumn walks the vectorColumns list and aligns each column's
// (knowledge_chunks.embedding and agent_memories.embedding) dimension to the configured one,
// making the config the single source of truth: switching the embedding model (which usually
// changes the dimension) only needs the user to edit ai.embedding.dimensions, and the columns
// follow automatically on startup without editing the migration or rebuilding the index by hand.
// The returned reconciled flag means "at least one column was changed".
func ReconcileEmbeddingColumn(ctx context.Context, db *gorm.DB, dimensions int) (reconciled bool, err error) {
	if db == nil {
		return false, nil
	}
	return reconcileVectorColumns(ctx, dimensions,
		func(c context.Context, table, column string) (int, bool, error) {
			return currentColumnDimension(c, db, table, column)
		},
		func(c context.Context, sql string) error { return db.WithContext(c).Exec(sql).Error },
	)
}

// reconcileVectorColumns 是多列校准内核：按清单逐列判定并在维度不一致时改动该列；任一列出错
// 即返回该错误（此前已完成的列不回滚），reconciled 表示至少一列被改动过。表/列不存在时该列
// 静默跳过。
// reconcileVectorColumns is the multi-column core: it walks the list and changes only the columns
// whose dimension differs. Any column error is returned immediately (previously completed columns
// are not rolled back) and reconciled means at least one column changed. A missing table/column is
// skipped silently.
func reconcileVectorColumns(
	ctx context.Context,
	dimensions int,
	currentDim func(context.Context, string, string) (int, bool, error),
	exec func(context.Context, string) error,
) (reconciled bool, err error) {
	if dimensions <= 0 {
		return false, nil
	}
	for _, vc := range vectorColumns {
		changed, err := reconcileVectorColumn(ctx, vc, dimensions,
			func(c context.Context) (int, bool, error) { return currentDim(c, vc.table, vc.column) },
			exec,
		)
		if err != nil {
			return reconciled, err
		}
		if changed {
			reconciled = true
		}
	}
	return reconciled, nil
}

// reconcileEmbeddingColumn 是单列可测试内核，保留既有签名与行为：委托 reconcileVectorColumn 并
// 以知识库列元数据执行。既有单测直接驱动本函数，行为不得改变。
// reconcileEmbeddingColumn is the single-column testable core, keeping its original signature and
// behavior: it delegates to reconcileVectorColumn with the knowledge-column metadata. Existing
// tests drive this function directly and its behavior must not change.
func reconcileEmbeddingColumn(
	ctx context.Context,
	dimensions int,
	currentDim func(context.Context) (int, bool, error),
	exec func(context.Context, string) error,
) (reconciled bool, err error) {
	return reconcileVectorColumn(ctx,
		vectorColumn{table: chunkEmbeddingTable, column: chunkEmbeddingColumn, index: chunkEmbeddingIndex},
		dimensions, currentDim, exec)
}

// reconcileVectorColumn 是逐列内核：维度判定与 DDL 执行分别经 currentDim / exec 注入。仅当实际
// 列维度与目标维度不一致时才改动：删 HNSW 索引 → 清除旧向量 → ALTER TYPE vector(N) → 重建
// HNSW 索引；表/列不存在（found=false）时静默跳过。
// reconcileVectorColumn is the per-column core: dimension inspection and DDL execution are
// injected via currentDim / exec. It changes anything only when the current column dimension
// differs from the target: drop the HNSW index → clear stale vectors → ALTER TYPE vector(N) →
// rebuild the HNSW index; a missing table/column (found=false) is skipped silently.
func reconcileVectorColumn(
	ctx context.Context,
	vc vectorColumn,
	dimensions int,
	currentDim func(context.Context) (int, bool, error),
	exec func(context.Context, string) error,
) (reconciled bool, err error) {
	if dimensions <= 0 {
		return false, nil
	}
	current, found, err := currentDim(ctx)
	if err != nil || !found {
		return false, err
	}
	if current == dimensions {
		return false, nil
	}
	// 先删 HNSW 索引：ALTER TYPE 不能用于带索引的列
	// Drop the HNSW index first: ALTER TYPE cannot run on an indexed column.
	if err := exec(
		ctx,
		fmt.Sprintf(
			`DROP INDEX IF EXISTS %s`,
			vc.index,
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	// 清空旧向量：维度变化后旧向量长度不符且由旧模型产生，无法转写，必须清空后重索引
	// Clear stale vectors: after a dimension change the old vectors have the wrong length and
	// came from the old model, so they cannot be recast and must be cleared and reindexed.
	if err := exec(
		ctx,
		fmt.Sprintf(
			`DELETE FROM %s`,
			vc.table,
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	if err := exec(
		ctx,
		fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE vector(%s)`,
			vc.table,
			vc.column,
			strconv.Itoa(dimensions),
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	// 重建 HNSW 索引（向量列维度变化后须按新维度重建）
	// Rebuild the HNSW index (it must be rebuilt against the new column dimension).
	if err := exec(
		ctx,
		fmt.Sprintf(
			`CREATE INDEX %s ON %s USING hnsw (%s vector_cosine_ops)`,
			vc.index,
			vc.table,
			vc.column,
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	return true, nil
}

// CheckEmbeddingDimension 按 vectorColumns 清单逐列校验（knowledge_chunks.embedding 与
// agent_memories.embedding）维度与配置指定维度一致；某列可读且不一致时返回提示运行 migrate
// 子命令的英文错误，并点名是哪张表哪列。表或列不存在（数据库尚未建表）或维度读不出时返回 nil，
// 交由迁移负责建列。用于服务启动时的强校验：维度不一致即中止启动，用户需自行执行 migrate
// 重建列，而非在启动时自动改动数据库结构。
// CheckEmbeddingDimension walks the vectorColumns list and verifies each column's
// (knowledge_chunks.embedding and agent_memories.embedding) dimension against the configured
// one, returning an English error naming the table and column that directs the user to run the
// migrate subcommand on mismatch. It returns nil when a table or column is missing (the database
// has not been migrated yet) or the dimension is unreadable, leaving column creation to the
// migration. It serves as the strict startup check: a dimension mismatch aborts startup and the
// user is expected to reconcile the columns via the migrate subcommand, rather than altering the
// schema automatically at startup.
func CheckEmbeddingDimension(ctx context.Context, db *gorm.DB, dimensions int) error {
	if db == nil || dimensions <= 0 {
		return nil
	}
	for _, vc := range vectorColumns {
		current, found, err := currentColumnDimension(ctx, db, vc.table, vc.column)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if current != dimensions {
			return errors.Newf(
				"embedding column dimension mismatch on %s.%s: column is %d, config is %d; run the migrate subcommand to reconcile",
				vc.table, vc.column, current, dimensions,
			)
		}
	}
	return nil
}

// ChunkEmbeddingDimension 返回 knowledge_chunks.embedding 列的 pgvector 维度与是否可读，
// 供状态自检等外部共用与校准一致的读取来源。
// ChunkEmbeddingDimension returns the pgvector dimension of the knowledge_chunks.embedding
// column and whether it was readable, sharing the same read source as the reconcile step for
// the status self-check and others.
func ChunkEmbeddingDimension(ctx context.Context, db *gorm.DB) (int, bool, error) {
	return currentEmbeddingDimension(ctx, db)
}

// MemoryEmbeddingDimension 返回 agent_memories.embedding 列的 pgvector 维度与是否可读，
// 供状态自检等外部比较记忆列维度与配置维度，读取来源与校准一致。
// MemoryEmbeddingDimension returns the pgvector dimension of the agent_memories.embedding
// column and whether it was readable, letting the status self-check and others compare the
// memory-column dimension against the configured one through the same read source as the
// reconcile step.
func MemoryEmbeddingDimension(ctx context.Context, db *gorm.DB) (int, bool, error) {
	return currentColumnDimension(ctx, db, memoryEmbeddingTable, memoryEmbeddingColumn)
}

// currentEmbeddingDimension 读取 knowledge_chunks.embedding 列的 pgvector 维度；
// 返回 (维度, 是否读到, 错误)。表或列不存在时返回 (0, false, nil)。
// currentEmbeddingDimension reads the pgvector dimension of the knowledge_chunks.embedding
// column; it returns (dimension, found, error). A missing table or column yields
// (0, false, nil).
func currentEmbeddingDimension(ctx context.Context, db *gorm.DB) (int, bool, error) {
	return currentColumnDimension(ctx, db, chunkEmbeddingTable, chunkEmbeddingColumn)
}

// currentColumnDimension 读取指定（表, 列）的 pgvector 维度；返回 (维度, 是否读到, 错误)。
// 表或列不存在时返回 (0, false, nil)。
// currentColumnDimension reads the pgvector dimension of the given (table, column); it returns
// (dimension, found, error). A missing table or column yields (0, false, nil).
func currentColumnDimension(ctx context.Context, db *gorm.DB, table, column string) (int, bool, error) {
	var columnType string
	if err := db.WithContext(ctx).Raw(
		`
			SELECT
				format_type(a.atttypid, a.atttypmod)
			FROM pg_attribute a
			JOIN pg_class c
			ON c.oid = a.attrelid
			WHERE c.relname = ? AND a.attname = ?`,
		table, column,
	).Scan(&columnType).Error; err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return 0, false, nil
		}
		return 0, false, errors.WithStack(err)
	}
	if columnType == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(columnType, "vector("), ")"))
	if err != nil {
		return 0, false, errors.Newf("unexpected embedding column type %q", columnType)
	}
	return n, true, nil
}

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

// 知识库向量检索依赖的 pgvector 表/列/索引元数据，与 internal/migrations/core/001_init.sql 对齐。
// pgvector table/column/index metadata the knowledge retrieval relies on, aligned with
// internal/migrations/core/001_init.sql.
const (
	chunkEmbeddingTable  = "knowledge_chunks"
	chunkEmbeddingColumn = "embedding"
	chunkEmbeddingIndex  = "idx_knowledge_chunks_embedding"
)

// ReconcileEmbeddingColumn 将 knowledge_chunks.embedding 列的维度校准为配置指定的维度，
// 使「配置为唯一事实来源」：更换 embedding 模型（通常伴随维度变化）时，用户只需改
// ai.embedding.dimensions，启动后数据库列会自动跟随，无需手工改迁移或重建索引。
// ReconcileEmbeddingColumn aligns the dimension of the knowledge_chunks.embedding column to
// the configured dimension, making the config the single source of truth: switching the
// embedding model (which usually changes the dimension) only needs the user to edit
// ai.embedding.dimensions, and the column follows automatically on startup without editing the
// migration or rebuilding the index by hand.
func ReconcileEmbeddingColumn(ctx context.Context, db *gorm.DB, dimensions int) (reconciled bool, err error) {
	if db == nil {
		return false, nil
	}
	return reconcileEmbeddingColumn(ctx, dimensions,
		func(c context.Context) (int, bool, error) { return currentEmbeddingDimension(c, db) },
		func(c context.Context, sql string) error { return db.WithContext(c).Exec(sql).Error },
	)
}

// reconcileEmbeddingColumn 是 ReconcileEmbeddingColumn 的可测试内核：维度判定与 DDL
// 执行分别经 currentDim / exec 注入。仅当实际列维度与目标维度不一致时才改动：删 HNSW 索引
// → 清除旧向量 → ALTER TYPE vector(N) → 重建 HNSW 索引；表/列不存在时静默跳过。
// reconcileEmbeddingColumn is the testable core of ReconcileEmbeddingColumn: dimension
// inspection and DDL execution are injected via currentDim / exec. It changes anything only
// when the current column dimension differs from the target: drop the HNSW index → clear stale
// vectors → ALTER TYPE vector(N) → rebuild the HNSW index; a missing table/column is skipped.
func reconcileEmbeddingColumn(
	ctx context.Context,
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
			chunkEmbeddingIndex,
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
			chunkEmbeddingTable,
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	if err := exec(
		ctx,
		fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE vector(%s)`,
			chunkEmbeddingTable,
			chunkEmbeddingColumn,
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
			chunkEmbeddingIndex,
			chunkEmbeddingTable,
			chunkEmbeddingColumn,
		),
	); err != nil {
		return false, errors.WithStack(err)
	}
	return true, nil
}

// CheckEmbeddingDimension 校验 knowledge_chunks.embedding 列维度与配置指定维度一致；
// 不一致时返回提示运行 migrate 子命令的英文错误。表或列不存在（数据库尚未建表）或维度
// 读不出时返回 nil，交由迁移负责建列。用于服务启动时的强校验：维度不一致即中止启动，
// 用户需自行执行 migrate 重建列，而非在启动时自动改动数据库结构。
// CheckEmbeddingDimension verifies that the knowledge_chunks.embedding column dimension
// matches the configured dimension, returning an English error directing the user to run the
// migrate subcommand on mismatch. It returns nil when the table or column is missing (the
// database has not been migrated yet) or the dimension is unreadable, leaving column creation
// to the migration. It serves as the strict startup check: a dimension mismatch aborts startup
// and the user is expected to reconcile the column via the migrate subcommand, rather than
// altering the schema automatically at startup.
func CheckEmbeddingDimension(ctx context.Context, db *gorm.DB, dimensions int) error {
	if db == nil || dimensions <= 0 {
		return nil
	}
	current, found, err := currentEmbeddingDimension(ctx, db)
	if err != nil || !found {
		return err
	}
	if current != dimensions {
		return errors.Newf(
			"embedding column dimension mismatch: column is %d, config is %d; run the migrate subcommand to reconcile",
			current, dimensions,
		)
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

// currentEmbeddingDimension 读取 knowledge_chunks.embedding 列的 pgvector 维度；
// 返回 (维度, 是否读到, 错误)。表或列不存在时返回 (0, false, nil)。
// currentEmbeddingDimension reads the pgvector dimension of the knowledge_chunks.embedding
// column; it returns (dimension, found, error). A missing table or column yields
// (0, false, nil).
func currentEmbeddingDimension(ctx context.Context, db *gorm.DB) (int, bool, error) {
	var columnType string
	if err := db.WithContext(ctx).Raw(
		`
			SELECT 
				format_type(a.atttypid, a.atttypmod) 
			FROM pg_attribute a 
			JOIN pg_class c 
			ON c.oid = a.attrelid 
			WHERE c.relname = ? AND a.attname = ?`,
		chunkEmbeddingTable, chunkEmbeddingColumn,
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

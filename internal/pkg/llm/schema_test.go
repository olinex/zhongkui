package llm

import (
	// stdlib
	"context"
	"strings"
	"testing"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// runReconcile 以给定的当前列维度驱动 reconcileEmbeddingColumn 内核，并捕获实际执行的 DDL。
// runReconcile drives the reconcileEmbeddingColumn core with the given current column
// dimension, capturing the DDL statements actually issued.
func runReconcile(current int, found bool, scanErr error, target int) (reconciled bool, executed []string, err error) {
	currentDim := func(context.Context) (int, bool, error) {
		return current, found, scanErr
	}
	exec := func(_ context.Context, sql string) error {
		executed = append(executed, sql)
		return nil
	}
	reconciled, err = reconcileEmbeddingColumn(context.Background(), target, currentDim, exec)
	return reconciled, executed, err
}

func TestReconcileEmbeddingColumn_Reconciles(t *testing.T) {
	// 旧列 1536、目标 1024：应删索引、清向量、改维度、重建索引。
	reconciled, executed, err := runReconcile(1536, true, nil, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconciled {
		t.Fatalf("expected reconcile for mismatched dimension")
	}
	if len(executed) != 4 {
		t.Fatalf("expected 4 DDL statements, got %d: %v", len(executed), executed)
	}
	if !strings.Contains(executed[2], "TYPE vector(1024)") {
		t.Errorf("expected ALTER TYPE vector(1024), got %q", executed[2])
	}
	// 索引须先于 ALTER 删除、后于 ALTER 重建
	if !strings.HasPrefix(executed[0], "DROP INDEX") || !strings.HasPrefix(executed[3], "CREATE INDEX") {
		t.Errorf("expected DROP before ALTER and CREATE after ALTER, got %v", executed)
	}
}

func TestReconcileEmbeddingColumn_NoopWhenAligned(t *testing.T) {
	reconciled, executed, err := runReconcile(1024, true, nil, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconciled {
		t.Fatalf("expected no reconcile for aligned dimension")
	}
	if len(executed) != 0 {
		t.Fatalf("expected no DDL for aligned dimension, got %v", executed)
	}
}

func TestReconcileEmbeddingColumn_TableMissingSkipped(t *testing.T) {
	// 列/表不存在：currentDim 返回 (0,false,nil)，应静默跳过。
	reconciled, executed, err := runReconcile(0, false, nil, 1024)
	if err != nil {
		t.Fatalf("expected missing table to be skipped, got: %v", err)
	}
	if reconciled || len(executed) != 0 {
		t.Fatalf("expected silent skip for missing table, reconciled=%v executed=%v", reconciled, executed)
	}
}

func TestReconcileEmbeddingColumn_ScanErrorPropagates(t *testing.T) {
	scanErr := errors.New("connection refused")
	_, _, err := runReconcile(1536, true, scanErr, 1024)
	if err == nil {
		t.Fatalf("expected non-missing scan error to propagate, got nil")
	}
}

// columnState 描述测试中注入的某列维度读取行为。
// columnState describes the injected dimension-read behavior of one column in tests.
type columnState struct {
	dim   int
	found bool
	err   error
}

// columnKey 以「表.列」为键，便于测试按列注入维度读取行为。
// columnKey builds a "table.column" key so tests can inject per-column behavior.
func columnKey(table, column string) string { return table + "." + column }

// runMultiReconcile 以逐列注入的维度驱动 reconcileVectorColumns 多列内核并捕获实际执行的 DDL。
// runMultiReconcile drives the reconcileVectorColumns multi-column core with per-column injected
// dimensions, capturing the DDL statements actually issued.
func runMultiReconcile(states map[string]columnState, target int) (reconciled bool, executed []string, err error) {
	currentDim := func(_ context.Context, table, column string) (int, bool, error) {
		st := states[columnKey(table, column)]
		return st.dim, st.found, st.err
	}
	exec := func(_ context.Context, sql string) error {
		executed = append(executed, sql)
		return nil
	}
	reconciled, err = reconcileVectorColumns(context.Background(), target, currentDim, exec)
	return reconciled, executed, err
}

func TestReconcileVectorColumns_OnlyMismatchedColumnChanged(t *testing.T) {
	// 仅知识库列 1536→1024 不一致，记忆列已对齐：DDL 只应针对知识库表。
	states := map[string]columnState{
		columnKey(chunkEmbeddingTable, chunkEmbeddingColumn):   {dim: 1536, found: true},
		columnKey(memoryEmbeddingTable, memoryEmbeddingColumn): {dim: 1024, found: true},
	}
	reconciled, executed, err := runMultiReconcile(states, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconciled {
		t.Fatalf("expected reconcile for the mismatched knowledge column")
	}
	if len(executed) != 4 {
		t.Fatalf("expected 4 DDL statements for one column, got %d: %v", len(executed), executed)
	}
	for _, sql := range executed {
		if strings.Contains(sql, memoryEmbeddingTable) {
			t.Errorf("aligned memory column must not be touched, got DDL %q", sql)
		}
	}
}

func TestReconcileVectorColumns_MissingColumnSkipped(t *testing.T) {
	// 知识库表缺失（found=false）应静默跳过，记忆列照常校准。
	states := map[string]columnState{
		columnKey(chunkEmbeddingTable, chunkEmbeddingColumn):   {found: false},
		columnKey(memoryEmbeddingTable, memoryEmbeddingColumn): {dim: 512, found: true},
	}
	reconciled, executed, err := runMultiReconcile(states, 1024)
	if err != nil {
		t.Fatalf("expected missing table to be skipped, got: %v", err)
	}
	if !reconciled {
		t.Fatalf("expected the present memory column to be reconciled")
	}
	if len(executed) != 4 {
		t.Fatalf("expected 4 DDL statements for the memory column, got %d: %v", len(executed), executed)
	}
	for _, sql := range executed {
		if strings.Contains(sql, chunkEmbeddingTable) {
			t.Errorf("missing knowledge column must be skipped, got DDL %q", sql)
		}
	}
	if !strings.Contains(executed[2], "TYPE vector(1024)") {
		t.Errorf("expected ALTER TYPE vector(1024), got %q", executed[2])
	}
}

func TestReconcileVectorColumns_NoopWhenAllAligned(t *testing.T) {
	states := map[string]columnState{
		columnKey(chunkEmbeddingTable, chunkEmbeddingColumn):   {dim: 1024, found: true},
		columnKey(memoryEmbeddingTable, memoryEmbeddingColumn): {dim: 1024, found: true},
	}
	reconciled, executed, err := runMultiReconcile(states, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconciled {
		t.Fatalf("expected no reconcile when all columns are aligned")
	}
	if len(executed) != 0 {
		t.Fatalf("expected no DDL when all columns are aligned, got %v", executed)
	}
}

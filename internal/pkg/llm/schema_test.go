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

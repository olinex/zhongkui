package agentkit

import (
	"context"
	"testing"
)

func TestMemoryCheckpointStoreLifecycle(t *testing.T) {
	store := NewMemoryCheckPointStore()
	ctx := context.Background()

	if _, ok, err := store.Get(ctx, "missing"); err != nil || ok {
		t.Fatalf("Get(missing) = ok:%v err:%v", ok, err)
	}

	payload := []byte("state")
	if err := store.Set(ctx, "cp", payload); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	payload[0] = 'X'
	got, ok, err := store.Get(ctx, "cp")
	if err != nil || !ok {
		t.Fatalf("Get() = ok:%v err:%v", ok, err)
	}
	if string(got) != "state" {
		t.Fatalf("Get() = %q, want an isolated copy", got)
	}

	if err := store.Delete(ctx, "cp"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get(ctx, "cp"); err != nil || ok {
		t.Fatalf("Get(after delete) = ok:%v err:%v", ok, err)
	}
	if err := store.Delete(ctx, "cp"); err != nil {
		t.Fatalf("Delete() must be idempotent: %v", err)
	}
}

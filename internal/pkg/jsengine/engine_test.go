// 本文件覆盖 jsengine 的公共契约：注册脚本在 Start 时执行、Do 串行访问、未启动/已关闭的拒绝、
// 回调错误透传与并发安全。

package jsengine_test

import (
	// 标准库
	"context"
	"errors"
	"sync"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
)

var (
	// probeOnce 保证探针脚本只登记一次（避免 -count 重复执行时触发重名 panic）。
	probeOnce sync.Once
)

// registerProbe 登记一段探针脚本：引擎启动后 globalThis.__probe 应为 "ready"。
func registerProbe() {
	probeOnce.Do(func() {
		jsengine.RegisterScript("test.probe", []byte(`globalThis.__probe = "ready";`))
	})
}

// startedEngine 返回已启动的引擎，并在测试结束时关闭。
func startedEngine(t *testing.T) *jsengine.Engine {
	t.Helper()
	registerProbe()
	engine := jsengine.New()
	if err := engine.Start(context.Background()); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Shutdown()
	})
	return engine
}

// TestEngine_StartRunsRegisteredScripts 覆盖「注册的脚本在 Start 时执行」。
func TestEngine_StartRunsRegisteredScripts(t *testing.T) {
	engine := startedEngine(t)
	if err := engine.Do(func(vm *jsengine.Runtime) error {
		if got := vm.Get("__probe"); got == nil || got.String() != "ready" {
			t.Errorf("__probe = %v, want ready", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
}

// TestEngine_DoBeforeStart 覆盖未启动即访问：应返回 ErrNotStarted。
func TestEngine_DoBeforeStart(t *testing.T) {
	engine := jsengine.New()
	if err := engine.Do(func(*jsengine.Runtime) error { return nil }); !errors.Is(err, jsengine.ErrNotStarted) {
		t.Errorf("Do error = %v, want ErrNotStarted", err)
	}
}

// TestEngine_ShutdownRejectsAccess 覆盖关闭后的访问与幂等关闭。
func TestEngine_ShutdownRejectsAccess(t *testing.T) {
	engine := startedEngine(t)
	if err := engine.Shutdown(); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
	if err := engine.Shutdown(); err != nil {
		t.Errorf("second Shutdown error = %v, want nil", err)
	}
	if err := engine.Do(func(*jsengine.Runtime) error { return nil }); !errors.Is(err, jsengine.ErrClosed) {
		t.Errorf("Do after shutdown error = %v, want ErrClosed", err)
	}
	if err := engine.Start(context.Background()); !errors.Is(err, jsengine.ErrClosed) {
		t.Errorf("Start after shutdown error = %v, want ErrClosed", err)
	}
}

// TestEngine_DoPropagatesCallbackError 覆盖回调错误原样透传。
func TestEngine_DoPropagatesCallbackError(t *testing.T) {
	engine := startedEngine(t)
	expected := errors.New("callback failed")
	if err := engine.Do(func(*jsengine.Runtime) error { return expected }); !errors.Is(err, expected) {
		t.Errorf("Do error = %v, want callback error", err)
	}
}

// TestEngine_ConcurrentDo 覆盖并发访问：引擎内部串行化，不应出现数据竞争或结果错乱。
func TestEngine_ConcurrentDo(t *testing.T) {
	engine := startedEngine(t)
	const workers = 8
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for index := 0; index < workers; index++ {
		go func() {
			defer waitGroup.Done()
			for round := 0; round < 20; round++ {
				if err := engine.Do(func(vm *jsengine.Runtime) error {
					if got := vm.Get("__probe").String(); got != "ready" {
						t.Errorf("__probe = %q, want ready", got)
					}
					return nil
				}); err != nil {
					t.Errorf("Do error = %v", err)
					return
				}
			}
		}()
	}
	waitGroup.Wait()
}

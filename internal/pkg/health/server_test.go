package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/status"
)

// depProbeOK 返回一个始终就绪（可达）的依赖探测项。
func depProbeOK(ctx context.Context) status.Status {
	return status.Status{Name: "redis", Category: "cache", Configured: true, Reachable: true}
}

// depProbeUnreachable 返回一个已配置但不可达的依赖探测项。
func depProbeUnreachable(ctx context.Context) status.Status {
	return status.Status{Name: "redis", Category: "cache", Configured: true, Reachable: false, Err: "dial tcp: i/o timeout"}
}

// newTestServer 构造一个用于测试的健康探针服务。
func newTestServer(reg *status.Registry) *Server {
	return NewServer(&Config{ReadyTimeout: time.Second, Enabled: true}, reg, nil)
}

// perform 用 buildMux() 对给定路径发起一次 GET 请求并返回响应。
func perform(s *Server, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.buildMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// TestStartupNotStarted 验证未上报启动时 /startupz 返回 503。
func TestStartupNotStarted(t *testing.T) {
	s := newTestServer(status.New())
	if rec := perform(s, "/startupz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/startupz before ReportStarted: got %d, want 503", rec.Code)
	}
}

// TestReadyNotStarted 验证仅开始监听但未上报启动时 /readyz 返回 503。
func TestReadyNotStarted(t *testing.T) {
	s := newTestServer(status.New())
	s.listening.Store(true)
	if rec := perform(s, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz before ReportStarted: got %d, want 503", rec.Code)
	}
}

// TestAllOK 验证依赖全部就绪时各探针端点返回 200。
func TestAllOK(t *testing.T) {
	reg := status.New()
	reg.Register(status.Spec{Name: "redis", Category: "cache", Probe: depProbeOK})
	s := newTestServer(reg)
	s.started.Store(true)
	s.listening.Store(true)

	t.Run("/healthz", func(t *testing.T) {
		if rec := perform(s, "/healthz"); rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
	})
	t.Run("/startupz", func(t *testing.T) {
		if rec := perform(s, "/startupz"); rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
	})
	t.Run("/readyz", func(t *testing.T) {
		if rec := perform(s, "/readyz"); rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
	})
}

// TestReadyUnreachable 验证依赖不可达时 /readyz 返回 503 并携带依赖状态。
func TestReadyUnreachable(t *testing.T) {
	reg := status.New()
	reg.Register(status.Spec{Name: "redis", Category: "cache", Probe: depProbeUnreachable})
	s := newTestServer(reg)
	s.started.Store(true)
	s.listening.Store(true)

	rec := perform(s, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz with unreachable dep: got %d, want 503", rec.Code)
	}

	var deps []status.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &deps); err != nil {
		t.Fatalf("failed to decode deps json: %v", err)
	}
	if len(deps) == 0 {
		t.Fatal("expected at least one dep status in body")
	}
	if got := deps[0].State(); got != "unreachable" {
		t.Fatalf("first dep State()=%q, want \"unreachable\"", got)
	}
}

// TestReadyDisabledDoesNotBlock 验证未配置的依赖不阻塞就绪判定。
func TestReadyDisabledDoesNotBlock(t *testing.T) {
	reg := status.New()
	reg.Register(status.Spec{Name: "redis", Category: "cache", Probe: depProbeOK})
	reg.Register(status.Spec{Name: "smtp", Category: "mail", Probe: func(ctx context.Context) status.Status {
		return status.Status{Name: "smtp", Category: "mail", Configured: false}
	}})
	s := newTestServer(reg)
	s.started.Store(true)
	s.listening.Store(true)

	if rec := perform(s, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("/readyz with disabled dep: got %d, want 200", rec.Code)
	}
}

// TestShuttingDown 验证关闭过程中 /healthz 与 /readyz 均返回 503。
func TestShuttingDown(t *testing.T) {
	reg := status.New()
	reg.Register(status.Spec{Name: "redis", Category: "cache", Probe: depProbeOK})
	s := newTestServer(reg)
	s.started.Store(true)
	s.listening.Store(true)
	s.shuttingDown.Store(true)

	t.Run("/healthz", func(t *testing.T) {
		if rec := perform(s, "/healthz"); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", rec.Code)
		}
	})
	t.Run("/readyz", func(t *testing.T) {
		if rec := perform(s, "/readyz"); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", rec.Code)
		}
	})
}

// TestAllStateReady 验证 allStateReady 对不可达依赖判否、对就绪/未配置依赖判真。
func TestAllStateReady(t *testing.T) {
	ok := status.Status{Name: "redis", Category: "cache", Configured: true, Reachable: true}
	unreachable := status.Status{Name: "redis", Category: "cache", Configured: true, Reachable: false}
	disabled := status.Status{Name: "smtp", Category: "mail", Configured: false}

	t.Run("unreachable blocks", func(t *testing.T) {
		if allStateReady([]status.Status{ok, unreachable, disabled}) {
			t.Fatal("expected false when a dep is unreachable")
		}
	})
	t.Run("ok/disabled pass", func(t *testing.T) {
		if !allStateReady([]status.Status{ok, disabled}) {
			t.Fatal("expected true when only ok/disabled deps present")
		}
	})
}

// TestSnapshot 验证状态快照包含启动标记、服务名、主机与进程 ID。
func TestSnapshot(t *testing.T) {
	s := NewServer(&Config{ReadyTimeout: time.Second}, status.New(), nil)
	s.serviceName = "test-service"
	s.started.Store(true)

	snap := s.Snapshot(context.Background())
	if !snap.Started {
		t.Fatal("snap.Started = false, want true")
	}
	if snap.ServiceName == "" {
		t.Error("snap.ServiceName is empty")
	}
	if snap.Host == "" {
		t.Error("snap.Host is empty")
	}
	if snap.PID == 0 {
		t.Error("snap.PID is 0")
	}
}

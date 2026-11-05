package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	otelx "zhongkuisec.com/zhongkui/internal/pkg/otel"
	"zhongkuisec.com/zhongkui/internal/pkg/status"
	"zhongkuisec.com/zhongkui/internal/pkg/version"
)

// Snapshot 服务运行状态的瞬时快照，供 etcd 上报与测试断言使用。
type Snapshot struct {
	ServiceName string    // 服务名 / service name
	InstanceID  string    // 实例 ID / instance identifier
	Host        string    // 主机名 / host name
	Version     string    // 服务版本 / service version
	PID         int       // 进程 ID / process id
	StartedAt   time.Time // 启动时刻 / start time
	Live        bool      // 存活 / live
	Ready       bool      // 就绪 / ready
	Started     bool      // 启动完成 / startup finished
}

// Server 项目全局统一的健康探针 HTTP 服务。
// 暴露 /healthz /readyz /startupz 三个 k8s 语义探针端点；就绪信号复用 *status.Registry；
// 可选地经 etcd 发布自身运行状态到服务目录。其内存模型参照 manager.RunHttpService
// （goroutine + errCh + ctx 取消优雅关闭）。
type Server struct {
	// cfg 健康探针服务配置。
	cfg *Config
	// reg 就绪探测注册表。
	reg *status.Registry

	// metrics 健康状态指标记录器。
	metrics *healthMetrics

	// serviceName 服务名，用于 etcd 目录键与日志标识。
	serviceName string
	// instanceID 实例标识（host-pid）。
	instanceID string
	// host 主机名。
	host string
	// pid 进程 ID。
	pid int

	// startTime 启动时刻。
	startTime time.Time
	// listening 是否正在监听。
	listening atomic.Bool
	// started 是否已完成启动。
	started atomic.Bool
	// shuttingDown 是否进入关机。
	shuttingDown atomic.Bool

	// regClient etcd 客户端，用于服务状态上报（可为空）。
	regClient *etcd.EtcdClient

	// srv 底层 HTTP 服务。
	srv *http.Server
}

// NewServer 创建一个健康探针服务。
// reg 为就绪探测注册表；etcdClient 可空（为空则跳过 etcd 服务状态上报）。
func NewServer(cfg *Config, reg *status.Registry, etcdClient *etcd.EtcdClient) *Server {
	reg.SetPerProbeTimeout(cfg.ReadyTimeout)
	host, _ := os.Hostname()
	s := &Server{
		cfg:       cfg,
		reg:       reg,
		metrics:   newHealthMetrics(),
		host:      host,
		pid:       os.Getpid(),
		regClient: etcdClient,
	}
	s.srv = &http.Server{Handler: s.buildMux()}
	return s
}

// buildMux 装配 k8s 探针路由；OTEL 未启用时不做追踪包装。
func (s *Server) buildMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleLive)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/startupz", s.handleStartup)
	if otelx.Env.Enabled {
		return otelhttp.NewHandler(mux, "zk.health")
	}
	return mux
}

// Start 启动健康探针服务并阻塞运行，直到 ctx 取消或服务出错返回。
// serviceName 用于 etcd 服务目录键与日志标识。启用 etcd 上报时同时启动注册循环，
// 并在退出时吊销租赁。
func (s *Server) Start(ctx context.Context, logger *logging.Logger, serviceName string) error {
	if !s.cfg.Enabled {
		<-ctx.Done()
		return nil
	}
	s.serviceName = serviceName
	s.pid = os.Getpid()
	s.instanceID = fmt.Sprintf("%s-%d", s.host, s.pid)
	s.startTime = time.Now()

	if s.regClient != nil {
		reg := newRegistrar(s.regClient, s, s.cfg.Registry)
		regCtx, regCancel := context.WithCancel(ctx)
		defer regCancel()
		go reg.run(regCtx)
		defer reg.revoke(context.Background())
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	s.srv.Addr = addr
	s.listening.Store(true)
	errCh := make(chan error, 1)
	go func() {
		logger.Ctx(ctx).
			Infof("health probe service listening on %s (service=%s instance=%s)", addr, serviceName, s.instanceID)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- errors.WithStack(err)
		}
	}()

	select {
	case <-ctx.Done():
		s.ReportShuttingDown()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutCtx); err != nil {
			return errors.WithStack(err)
		}
		s.listening.Store(false)
		return nil
	case err := <-errCh:
		s.listening.Store(false)
		return errors.WithStack(err)
	}
}

// ReportStarted 标记服务完成启动，即 /startupz 与 /readyz 转为就绪前提。
func (s *Server) ReportStarted() { s.started.Store(true) }

// ReportShuttingDown 标记服务进入关机，/healthz 与 /readyz 转为失败。
func (s *Server) ReportShuttingDown() { s.shuttingDown.Store(true) }

// ServiceName 返回当前服务名。
func (s *Server) ServiceName() string { return s.serviceName }

// InstanceID 返回当前实例 ID。
func (s *Server) InstanceID() string { return s.instanceID }

// Addr 返回探针服务监听地址（测试用）。
func (s *Server) Addr() string { return s.srv.Addr }

// Snapshot 计算并返回当前服务状态快照；内部运行就绪探测并发布状态指标。
func (s *Server) Snapshot(ctx context.Context) Snapshot {
	started := s.started.Load()
	shutting := s.shuttingDown.Load()
	live := s.listening.Load() && !shutting

	ready := started && !shutting
	var results []status.Status
	if ready {
		results = s.reg.Run(ctx)
		ready = allStateReady(results)
	}
	s.metrics.record(ctx, live, ready, started, results)

	snap := Snapshot{
		ServiceName: s.serviceName,
		InstanceID:  s.instanceID,
		Host:        s.host,
		Version:     version.Short(),
		PID:         s.pid,
		StartedAt:   s.startTime,
		Live:        live,
		Ready:       ready,
		Started:     started,
	}
	return snap
}

// allStateReady 就绪收敛：任一已配置但不可达依赖（State==unreachable)即不就绪；
// 未配置（disabled/ok）不阻断。
func allStateReady(states []status.Status) bool {
	for _, st := range states {
		if st.State() == "unreachable" {
			return false
		}
	}
	return true
}

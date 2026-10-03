package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/health"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 各服务的固定服务名，用于 etcd 服务目录键与日志标识。
// 注：capturer 不在此列——它不直连 etcd，其状态经 gRPC Feedback 由 reporter 代为写入 etcd。
const (
	HealthServiceNameWeb    = "web"
	HealthServiceNameGrpc   = "grpc"
	HealthServiceNameWorker = "worker"
)

// healthServerProvider 构建全局健康探针服务：复用依赖探测注册表做就绪聚合，
// 注入 etcd 客户端用于服务状态上报（缺失时跳过）。
func healthServerProvider(ctx context.Context) do.Provider[*health.Server] {
	return func(i do.Injector) (*health.Server, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := health.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if reg, err := NewDependencyRegistry(ctx, i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			var etcdClient *etcd.EtcdClient
			if ec, err := InvokeEtcdClient(i); err == nil {
				etcdClient = ec
			}
			return health.NewServer(cfg, reg, etcdClient), nil
		}
	}
}

// NewHealthServer 返回构建全局健康探针服务的 provider 工厂。
func NewHealthServer(ctx context.Context) do.Provider[*health.Server] {
	return healthServerProvider(ctx)
}

// InjectHealthServer 注册全局健康探针服务。
func InjectHealthServer(ctx context.Context, i do.Injector) {
	do.Provide(i, NewHealthServer(ctx))
}

// InvokeHealthServer 从注入器解析全局健康探针服务。
func InvokeHealthServer(ctx context.Context, i do.Injector) (*health.Server, error) {
	return do.Invoke[*health.Server](i)
}

// MustInvokeHealthServer 从注入器解析全局健康探针服务，失败时 panic。
func MustInvokeHealthServer(ctx context.Context, i do.Injector) *health.Server {
	return do.MustInvoke[*health.Server](i)
}

// HealthService 将健康探针服务适配为 runtime.Service，
// 供 runtime.Group 统一编排（worker）或命令侧 goroutine 启动（web/grpc）。
type HealthService struct {
	*health.Server
	// logger 结构化日志器，供健康服务运行期输出。
	logger *logging.Logger
	// name 服务名，用于 etcd 服务目录键与日志标识。
	name string
}

// NewHealthService 构建以其服务名运行的健康探针服务。
func NewHealthService(ctx context.Context, i do.Injector, serviceName string) (*HealthService, error) {
	srv, err := InvokeHealthServer(ctx, i)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	logger, err := InvokeDefaultLogger(i)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &HealthService{Server: srv, logger: logger, name: serviceName}, nil
}

// Start 标记完成启动后阻塞运行健康探针服务，直到 ctx 取消。
func (h *HealthService) Start(ctx context.Context) error {
	h.ReportStarted()
	return h.Server.Start(ctx, h.logger, h.name)
}

// Shutdown 由 Server.Start 的 ctx 取消路径统一完成优雅关闭，无需额外处理。
func (h *HealthService) Shutdown(_ context.Context) error {
	return nil
}

// Name 返回编排器使用的服务名。
func (h *HealthService) Name() string { return "health" }

// DescribeHealthConfig 返回健康探针配置描述。
func DescribeHealthConfig() config.ConfigDescriptions {
	return health.DescribeConfig()
}

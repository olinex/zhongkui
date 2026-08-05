package provider

import (
	"time"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

const (
	ReporterGrpcServerLabel = "grpc.reporter.server"
	ReporterGrpcClientLabel = "grpc.reporter.client"
	ReporterGrpcPrefix      = "reporter"
)

var (
	_ do.ShutdownerWithError = (*grpc.Client)(nil)
)

func grpcReporterAuthenticator(i do.Injector) map[string]grpc.Authenticator {
	if coreDB, err := InvokeCoreDatabase(i); err != nil {
		return nil
	} else if cacheRedis, err := InvokeCacheRedis(i); err != nil {
		return nil
	} else {
		pool := core.NewAccessKeyPool(coreDB, cacheRedis, time.Minute)
		accessKeyAuthenticator := grpc.NewAccessKeyAuthenticator(pool)
		return map[string]grpc.Authenticator{
			grpc.ProtocolAccessKeySecret: accessKeyAuthenticator,
		}
	}
}

func grpcServerProvider(name string) do.Provider[*grpc.Server] {
	return func(i do.Injector) (*grpc.Server, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := grpc.ExtractServerConfig(viper, name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，添加 gRPC 追踪的 stats handler
		server, err := grpc.NewServer(grpcReporterAuthenticator(i), cfg, otel.GrpcServerOptions()...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return server, nil
	}
}

func grpcClientProvider(name string) do.Provider[*grpc.Client] {
	return func(i do.Injector) (*grpc.Client, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := grpc.ExtractClientConfig(viper, name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，添加 gRPC 追踪的 stats handler
		client, err := grpc.NewClient(cfg, otel.GrpcClientOptions()...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return client, nil
	}
}

func NewReporterGrpcServer() do.Provider[*grpc.Server] {
	return grpcServerProvider(ReporterGrpcPrefix)
}

func NewReporterGrpcClient() do.Provider[*grpc.Client] {
	return grpcClientProvider(ReporterGrpcPrefix)
}

func InjectReporterGrpcServer(i do.Injector) {
	do.ProvideNamed(i, ReporterGrpcServerLabel, NewReporterGrpcServer())
}

func InjectReporterGrpcClient(i do.Injector) {
	do.ProvideNamedTransient(i, ReporterGrpcClientLabel, NewReporterGrpcClient())
}

func InvokeReporterGrpcServer(i do.Injector) (*grpc.Server, error) {
	return do.InvokeNamed[*grpc.Server](i, ReporterGrpcServerLabel)
}

func InvokeReporterGrpcClient(i do.Injector) (*grpc.Client, error) {
	return do.InvokeNamed[*grpc.Client](i, ReporterGrpcClientLabel)
}

func MustInvokeReporterGrpcServer(i do.Injector) *grpc.Server {
	return do.MustInvokeNamed[*grpc.Server](i, ReporterGrpcServerLabel)
}

func MustInvokeReporterGrpcClient(i do.Injector) *grpc.Client {
	return do.MustInvokeNamed[*grpc.Client](i, ReporterGrpcClientLabel)
}

func DescribeGrpcServerConfig() config.ConfigDescriptions {
	return grpc.DescribeServerConfig(ReporterGrpcPrefix)
}

func DescribeGrpcClientConfig() config.ConfigDescriptions {
	return grpc.DescribeClientConfig(ReporterGrpcPrefix)
}

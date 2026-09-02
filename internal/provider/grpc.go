package provider

import (
	"time"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	ReporterGrpcServerLabel = "grpc.server." + base.ReporterGrpcName
	ReporterGrpcClientLabel = "grpc.client." + base.ReporterGrpcName
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

func newGrpcServerProvider(name string) do.Provider[*grpc.Server] {
	return func(i do.Injector) (*grpc.Server, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := grpc.ExtractServerConfig(viper, validator.Validate, name); err != nil {
			return nil, errors.WithStack(err)
		} else if server, err := grpc.NewServer(grpcReporterAuthenticator(i), cfg, otel.GrpcServerOptions()...); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return server, nil
		}
	}
}

func newGrpcClientProvider(name string) do.Provider[*grpc.Client] {
	return func(i do.Injector) (*grpc.Client, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := grpc.ExtractClientConfig(viper, validator.Validate, name); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := grpc.NewClient(cfg, otel.GrpcClientOptions()...); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return client, nil
		}
	}
}

func NewReporterGrpcServer() do.Provider[*grpc.Server] {
	return newGrpcServerProvider(base.ReporterGrpcName)
}

func NewReporterGrpcClient() do.Provider[*grpc.Client] {
	return newGrpcClientProvider(base.ReporterGrpcName)
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
	return grpc.DescribeServerConfig(base.ReporterGrpcName)
}

func DescribeGrpcClientConfig() config.ConfigDescriptions {
	return grpc.DescribeClientConfig(base.ReporterGrpcName)
}

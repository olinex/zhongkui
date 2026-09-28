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

// reporter gRPC 服务端与客户端在依赖注入器中的命名标签。
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

// NewReporterGrpcServer 返回 reporter gRPC 服务端的 provider 工厂。
func NewReporterGrpcServer() do.Provider[*grpc.Server] {
	return newGrpcServerProvider(base.ReporterGrpcName)
}

// NewReporterGrpcClient 返回 reporter gRPC 客户端的 provider 工厂。
func NewReporterGrpcClient() do.Provider[*grpc.Client] {
	return newGrpcClientProvider(base.ReporterGrpcName)
}

// InjectReporterGrpcServer 向依赖注入器注册 reporter gRPC 服务端 provider。
func InjectReporterGrpcServer(i do.Injector) {
	do.ProvideNamed(i, ReporterGrpcServerLabel, NewReporterGrpcServer())
}

// InjectReporterGrpcClient 向依赖注入器注册 reporter gRPC 客户端 provider。
func InjectReporterGrpcClient(i do.Injector) {
	// 单例长连接：gRPC 官方推荐复用单条 ClientConn；且作为单例，
	// 进程优雅关闭时会被 do 的 ShutdownWithContext 调用 Client.Shutdown 关闭连接
	do.ProvideNamed(i, ReporterGrpcClientLabel, NewReporterGrpcClient())
}

// InvokeReporterGrpcServer 从依赖注入器解析 reporter gRPC 服务端，错误交由调用方处理。
func InvokeReporterGrpcServer(i do.Injector) (*grpc.Server, error) {
	return do.InvokeNamed[*grpc.Server](i, ReporterGrpcServerLabel)
}

// InvokeReporterGrpcClient 从依赖注入器解析 reporter gRPC 客户端，错误交由调用方处理。
func InvokeReporterGrpcClient(i do.Injector) (*grpc.Client, error) {
	return do.InvokeNamed[*grpc.Client](i, ReporterGrpcClientLabel)
}

// MustInvokeReporterGrpcServer 从依赖注入器解析 reporter gRPC 服务端，失败时 panic。
func MustInvokeReporterGrpcServer(i do.Injector) *grpc.Server {
	return do.MustInvokeNamed[*grpc.Server](i, ReporterGrpcServerLabel)
}

// MustInvokeReporterGrpcClient 从依赖注入器解析 reporter gRPC 客户端，失败时 panic。
func MustInvokeReporterGrpcClient(i do.Injector) *grpc.Client {
	return do.MustInvokeNamed[*grpc.Client](i, ReporterGrpcClientLabel)
}

// DescribeGrpcServerConfig 返回 reporter gRPC 服务端的配置项描述。
func DescribeGrpcServerConfig() config.ConfigDescriptions {
	return grpc.DescribeServerConfig(base.ReporterGrpcName)
}

// DescribeGrpcClientConfig 返回 reporter gRPC 客户端的配置项描述。
func DescribeGrpcClientConfig() config.ConfigDescriptions {
	return grpc.DescribeClientConfig(base.ReporterGrpcName)
}

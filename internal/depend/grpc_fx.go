package depend

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/grpc"
)

const (
	REPORTER_GRPC = "reporter"
)

type GrpcServerFxIn struct {
	fx.In
	V              *viper.Viper
	Authenticators map[string]grpc.Authenticator
}

type GrpcClientFxIn struct {
	fx.In
	Lc fx.Lifecycle
	V  *viper.Viper
}

type ReporterGrpcServerFxIn struct {
	fx.In
	ReporterGrpcServer *grpc.Server `name:"reporter"`
}

type ReporterGrpcClientFxIn struct {
	fx.In
	ReporterGrpcClient *grpc.Client `name:"reporter"`
}

func newGrpcServer(name string) func(in GrpcServerFxIn) *grpc.Server {
	return func(in GrpcServerFxIn) *grpc.Server {
		if cfg, err := grpc.ExtractServerConfig(in.V, name); err != nil {
			panic(err)
		} else if server, err := grpc.NewServer(in.Authenticators, cfg); err != nil {
			panic(err)
		} else {
			return server
		}
	}
}

func newGrpcClient(name string) func(in GrpcClientFxIn) *grpc.Client {
	return func(in GrpcClientFxIn) *grpc.Client {
		if cfg, err := grpc.ExtractClientConfig(in.V, name); err != nil {
			panic(err)
		} else if client, err := grpc.NewClient(cfg); err != nil {
			panic(err)
		} else {
			in.Lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					return client.Close()
				},
			})
			return client
		}
	}
}

func grpcServerAnnotate(name string) any {
	return fx.Annotate(newGrpcServer(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func grpcClientAnnotate(name string) any {
	return fx.Annotate(newGrpcClient(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func ReporterGrpcServerAnnotate() any {
	return grpcServerAnnotate(REPORTER_GRPC)
}

func ReporterGrpcClientAnnotate() any {
	return grpcClientAnnotate(REPORTER_GRPC)
}

func DescribeGrpcServerConfig() config.ConfigDescriptions {
	return grpc.DescribeClientConfig(REPORTER_GRPC)
}

func DescribeGrpcClientConfig() config.ConfigDescriptions {
	return grpc.DescribeServerConfig(REPORTER_GRPC)
}

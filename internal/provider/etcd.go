package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/otel"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

func etcdClientProvider(ctx context.Context) do.Provider[*etcd.EtcdClient] {
	return func(i do.Injector) (*etcd.EtcdClient, error) {
		if logger, err := InvokeEtcdLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := etcd.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else {
			client, err := etcd.NewEtcdClient(ctx, cfg, logger, otel.EtcdDialOptions()...)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return client, nil
		}
	}
}

// NewEtcdClient 返回 etcd 客户端的依赖注入 provider 工厂。
func NewEtcdClient(ctx context.Context) do.Provider[*etcd.EtcdClient] {
	return etcdClientProvider(ctx)
}

// InjectEtcdClient 向依赖注入器注册 etcd 客户端 provider。
func InjectEtcdClient(ctx context.Context, i do.Injector) {
	do.Provide(i, NewEtcdClient(ctx))
}

// InvokeEtcdClient 从依赖注入器解析 etcd 客户端，错误由调用方处理。
func InvokeEtcdClient(i do.Injector) (*etcd.EtcdClient, error) {
	return do.Invoke[*etcd.EtcdClient](i)
}

// MustInvokeEtcdClient 从依赖注入器解析 etcd 客户端，失败时 panic（仅用于启动关键依赖）。
func MustInvokeEtcdClient(i do.Injector) *etcd.EtcdClient {
	return do.MustInvoke[*etcd.EtcdClient](i)
}

// DescribeEtcdConfig 返回 etcd 配置项的元描述列表。
func DescribeEtcdConfig() config.ConfigDescriptions {
	return etcd.DescribeConfig()
}

package provider

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/otel"
)

func etcdClientProvider(ctx context.Context) do.Provider[*etcd.EtcdClient] {
	return func(i do.Injector) (*etcd.EtcdClient, error) {
		if logger, err := InvokeEtcdLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := etcd.ExtractConfig(viper); err != nil {
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

func NewEtcdClient(ctx context.Context) do.Provider[*etcd.EtcdClient] {
	return etcdClientProvider(ctx)
}

func InjectEtcdClient(ctx context.Context, i do.Injector) {
	do.Provide(i, NewEtcdClient(ctx))
}

func InvokeEtcdClient(i do.Injector) (*etcd.EtcdClient, error) {
	return do.Invoke[*etcd.EtcdClient](i)
}

func MustInvokeEtcdClient(i do.Injector) *etcd.EtcdClient {
	return do.MustInvoke[*etcd.EtcdClient](i)
}

func DescribeEtcdConfig() config.ConfigDescriptions {
	return etcd.DescribeConfig()
}

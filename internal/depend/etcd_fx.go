package depend

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
)

const (
	MANAGE_ETCD = "manage"
)

type EtcdFxIn struct {
	fx.In
	EtcdLoggerFxIn
	V   *viper.Viper
	Ctx context.Context
}

type ManageEtcdFxIn struct {
	fx.In
	ManageEtcd *etcd.EtcdClient `name:"manage"`
}

type PipelineEtcdFxIn struct {
	fx.In
	PipelineEtcd *etcd.EtcdClient `name:"pipeline"`
}

func newEtcdClient(name string) func(in EtcdFxIn) *etcd.EtcdClient {
	return func(in EtcdFxIn) *etcd.EtcdClient {
		if cfg, err := etcd.ExtractConfig(in.V, name); err != nil {
			panic(err)
		} else if client, err := etcd.NewEtcdClient(in.Ctx, cfg, in.ETCDLogger); err != nil {
			panic(err)
		} else {
			return client
		}
	}
}

func etcdAnnotate(name string) any {
	return fx.Annotate(newEtcdClient(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func ManageEtcdAnnotate() any {
	return etcdAnnotate(MANAGE_ETCD)
}

func DescribeEtcdConfig() config.ConfigDescriptions {
	return etcd.DescribeConfig(MANAGE_ETCD)
}

package provider

import (
	"context"
	"io"

	"github.com/samber/do/v2"

	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

func newTagEngineProvider() do.Provider[*tagengine.Engine] {
	return func(i do.Injector) (*tagengine.Engine, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if logger, err := InvokeDatabaseLogger(i); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeEtcdClient(i); err != nil {
			return nil, errors.WithStack(err)
		} else if pool, err := InvokeSQLiteVTabPools(i); err != nil {
			return nil, errors.WithStack(err)
		} else if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := tagengine.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if s3Client, err := InvokeS3(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			// 注入按命名空间池：tagengine 作为消费方自行解析其依赖，
			// 而非由 sqlitevtab 的 provider 反向注入，保持依赖方向为
			// provider → tagengine → sqlitevtab。引擎求值时按命名空间
			// 绑定对应池到求值 ctx，供 sqlite.query 查询裸名 vtable。
			// 绑定 etcd 客户端：引擎作为 runtime.Service 可启动协程监听
			// 规则变更并对外广播变更事件，保证多实例下缓存一致。
			// 绑定敏感字段运行时：crypto.reveal 经 Zkcn 读取并解密 S3Crypto 正文。
			loader := func(ctx context.Context, objectKey string, cryptoKey []byte) (io.ReadCloser, error) {
				return s3.NewZkcn(s3Client, objectKey, cryptoKey, 0).Load(ctx)
			}
			return tagengine.NewEngine(db, logger, client,
				tagengine.WithDatasourcePools(pool),
				tagengine.WithSensitiveRuntime(loader, cfg.S3CryptoMaxBytes),
			), nil
		}
	}
}

// InjectTagEngine 向依赖注入器注册标签识别引擎 provider。
func InjectTagEngine(i do.Injector) {
	do.Provide(i, newTagEngineProvider())
}

// InvokeTagEngine 从依赖注入器解析标签识别引擎，错误交由调用方处理。
func InvokeTagEngine(i do.Injector) (*tagengine.Engine, error) {
	return do.Invoke[*tagengine.Engine](i)
}

// MustInvokeTagEngine 从依赖注入器解析标签识别引擎，失败时 panic。
func MustInvokeTagEngine(i do.Injector) *tagengine.Engine {
	return do.MustInvoke[*tagengine.Engine](i)
}

// DescribeTagEngineConfig 返回标签引擎配置项的元描述列表。
func DescribeTagEngineConfig() config.ConfigDescriptions {
	return tagengine.DescribeConfig()
}

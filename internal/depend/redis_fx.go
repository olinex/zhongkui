package depend

import (
	"context"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

const (
	CACHE_REDIS    = "cache"
	PIPELINE_REDIS = "pipeline"
)

type RedisFxIn struct {
	fx.In
	Lc fx.Lifecycle
	V  *viper.Viper
}

type CacheRedisFxIn struct {
	fx.In
	CacheRedis *redis.Client `name:"cache"`
}

type PipelineRedisFxIn struct {
	fx.In
	PipelineRedis *redis.Client `name:"pipeline"`
}

func newRedis(name string) func(in RedisFxIn) *redis.Client {
	return func(in RedisFxIn) *redis.Client {
		if cfg, err := redis.ExtractConfig(in.V, name); err != nil {
			panic(err)
		} else {
			cli := redis.NewClient(cfg)
			in.Lc.Append(fx.Hook{
				OnStop: func(ctx context.Context) error {
					return cli.Close()
				},
			})
			return cli
		}
	}
}

func redisAnnotate(name string) any {
	return fx.Annotate(newRedis(name), fx.ResultTags(fmt.Sprintf(`name:"%s"`, name)))
}

func CacheRedisAnnotate() any {
	return redisAnnotate(CACHE_REDIS)
}

func PipelineRedisAnnotate() any {
	return redisAnnotate(PIPELINE_REDIS)
}

func DescribeRedisConfig() config.ConfigDescriptions {
	return redis.DescribeConfig(CACHE_REDIS, PIPELINE_REDIS)
}

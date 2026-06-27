package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/redis"
)

const (
	CACHE_REDIS_LABEL     = "cache-redis"
	PIPELINE_REDIS_LABEL  = "pipeline-redis"
	CACHE_REDIS_PREFIX    = "cache"
	PIPELINE_REDIS_PREFIX = "pipeline"
)

func redisProvider(name string) do.Provider[*redis.Client] {
	return func(i do.Injector) (*redis.Client, error) {
		viper, err := InvokeViper(i)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, err := redis.ExtractConfig(viper, name)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		// 如果 OTEL 已启用，为 Redis 添加链路追踪和指标收集
		client := redis.NewClient(cfg)
		if err := otel.InjectRedisClient(client); err != nil {
			return nil, errors.WithStack(err)
		}
		return client, nil
	}
}

func NewCacheRedis() do.Provider[*redis.Client] {
	return redisProvider(CACHE_REDIS_PREFIX)
}

func NewPipelineRedis() do.Provider[*redis.Client] {
	return redisProvider(PIPELINE_REDIS_PREFIX)
}

func InjectCacheRedis(i do.Injector) {
	do.ProvideNamed(i, CACHE_REDIS_LABEL, NewCacheRedis())
}

func InjectPipelineRedis(i do.Injector) {
	do.ProvideNamed(i, PIPELINE_REDIS_LABEL, NewPipelineRedis())
}

func InvokeCacheRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, CACHE_REDIS_LABEL)
}

func InvokePipelineRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, PIPELINE_REDIS_LABEL)
}

func DescribeRedisConfig() config.ConfigDescriptions {
	return redis.DescribeConfig(CACHE_REDIS_PREFIX, PIPELINE_REDIS_PREFIX)
}

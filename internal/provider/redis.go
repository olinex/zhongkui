package provider

import (
	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/config"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/otel"
	"zhongkui.com/zhongkui/internal/pkg/redis"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

const (
	CacheRedisLabel    = "redis." + base.CacheRedisName
	PipelineRedisLabel = "redis." + base.PipelineRedisName
)

func redisProvider(name string) do.Provider[*redis.Client] {
	return func(i do.Injector) (*redis.Client, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := redis.ExtractConfig(viper, validator.Validate, name); err != nil {
			return nil, errors.WithStack(err)
		} else {
			client := redis.NewClient(cfg)
			if err := otel.InjectRedisClient(client); err != nil {
				return nil, errors.WithStack(err)
			}
			return client, nil
		}
	}
}

func NewCacheRedis() do.Provider[*redis.Client] {
	return redisProvider(base.CacheRedisName)
}

func NewPipelineRedis() do.Provider[*redis.Client] {
	return redisProvider(base.PipelineRedisName)
}

func InjectCacheRedis(i do.Injector) {
	do.ProvideNamed(i, CacheRedisLabel, NewCacheRedis())
}

func InjectPipelineRedis(i do.Injector) {
	do.ProvideNamed(i, PipelineRedisLabel, NewPipelineRedis())
}

func InvokeCacheRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, CacheRedisLabel)
}

func InvokePipelineRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, PipelineRedisLabel)
}

func DescribeRedisConfig() config.ConfigDescriptions {
	return redis.DescribeConfig(base.CacheRedisName, base.PipelineRedisName)
}

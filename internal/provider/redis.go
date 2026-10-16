package provider

import (
	"github.com/samber/do/v2"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/otel"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 缓存与管道 Redis 客户端在依赖注入器中的命名标签。
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

// NewCacheRedis 返回缓存 Redis 客户端的 provider 工厂。
func NewCacheRedis() do.Provider[*redis.Client] {
	return redisProvider(base.CacheRedisName)
}

// NewPipelineRedis 返回管道 Redis 客户端的 provider 工厂。
func NewPipelineRedis() do.Provider[*redis.Client] {
	return redisProvider(base.PipelineRedisName)
}

// InjectCacheRedis 向依赖注入器注册缓存 Redis 客户端 provider。
func InjectCacheRedis(i do.Injector) {
	do.ProvideNamed(i, CacheRedisLabel, NewCacheRedis())
}

// InjectPipelineRedis 向依赖注入器注册管道 Redis 客户端 provider。
func InjectPipelineRedis(i do.Injector) {
	do.ProvideNamed(i, PipelineRedisLabel, NewPipelineRedis())
}

// InvokeCacheRedis 从依赖注入器解析缓存 Redis 客户端，错误交由调用方处理。
func InvokeCacheRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, CacheRedisLabel)
}

// MustInvokeCacheRedis 从依赖注入器解析缓存 Redis 客户端；失败时 panic（仅用于启动关键依赖）。
func MustInvokeCacheRedis(i do.Injector) *redis.Client {
	return do.MustInvokeNamed[*redis.Client](i, CacheRedisLabel)
}

// InvokePipelineRedis 从依赖注入器解析管道 Redis 客户端，错误交由调用方处理。
func InvokePipelineRedis(i do.Injector) (*redis.Client, error) {
	return do.InvokeNamed[*redis.Client](i, PipelineRedisLabel)
}

// DescribeRedisConfig 返回缓存与管道 Redis 的配置项描述。
func DescribeRedisConfig() config.ConfigDescriptions {
	return redis.DescribeConfig(base.CacheRedisName, base.PipelineRedisName)
}

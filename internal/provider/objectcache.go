package provider

import (
	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// newObjectCacheProvider 构建共享的通用对象缓存（s3.ObjectCache）：依次注入 Viper 配置、
// 经 s3.ExtractConfig 解析并校验，再注入 S3 客户端后实例化。该实例为进程内唯一共享实例，
// web 侧用于知识库原件的上传/删除与流式下载，worker 侧用于把原件 Materialize 为本地文件后解析。
func newObjectCacheProvider() do.Provider[*s3.ObjectCache] {
	return func(i do.Injector) (*s3.ObjectCache, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := s3.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeS3(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return s3.NewObjectCache(client, *cfg), nil
		}
	}
}

// NewObjectCache 提供共享的通用对象缓存 provider（唯一实例）。
func NewObjectCache() do.Provider[*s3.ObjectCache] {
	return newObjectCacheProvider()
}

// InjectObjectCache 注册共享的通用对象缓存 provider。
func InjectObjectCache(i do.Injector) {
	do.Provide(i, NewObjectCache())
}

// InvokeObjectCache 取得共享的通用对象缓存实例。
func InvokeObjectCache(i do.Injector) (*s3.ObjectCache, error) {
	return do.Invoke[*s3.ObjectCache](i)
}

// MustInvokeObjectCache panic 版本取得共享的通用对象缓存实例。
func MustInvokeObjectCache(i do.Injector) *s3.ObjectCache {
	cache, err := InvokeObjectCache(i)
	if err != nil {
		panic(err)
	}
	return cache
}

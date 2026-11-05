package provider

import (
	// 标准库
	"time"

	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	coremodel "zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// AccessKeyPoolTTL 是访问密钥签名令牌的防重放窗口：时间戳超出该窗口或 nonce 在窗口内重复
// 均视为重放。Web 接口层与 gRPC 认证器共用同一窗口。
const AccessKeyPoolTTL = time.Minute

// newAccessKeyPoolProvider 构造访问密钥池 provider：依赖 core 数据库与缓存 Redis。
func newAccessKeyPoolProvider() do.Provider[*coremodel.AccessKeyPool] {
	return func(i do.Injector) (*coremodel.AccessKeyPool, error) {
		if db, err := InvokeCoreDatabase(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cache, err := InvokeCacheRedis(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return coremodel.NewAccessKeyPool(db, cache, AccessKeyPoolTTL), nil
		}
	}
}

// InjectAccessKeyPool 向依赖注入器注册访问密钥池 provider。
func InjectAccessKeyPool(i do.Injector) {
	do.Provide(i, newAccessKeyPoolProvider())
}

// InvokeAccessKeyPool 从依赖注入器解析访问密钥池，错误交由调用方处理。
func InvokeAccessKeyPool(i do.Injector) (*coremodel.AccessKeyPool, error) {
	return do.Invoke[*coremodel.AccessKeyPool](i)
}

// MustInvokeAccessKeyPool 从依赖注入器解析访问密钥池；失败时 panic（仅用于启动关键依赖）。
func MustInvokeAccessKeyPool(i do.Injector) *coremodel.AccessKeyPool {
	return do.MustInvoke[*coremodel.AccessKeyPool](i)
}

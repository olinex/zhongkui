package provider

import (
	// third-party
	"github.com/samber/do/v2"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// newSQLiteVTabPoolsProvider 创建按命名空间隔离的 sqlite vtable 池持有者（vtab.Pools）。
// 每个命名空间独立持有一个 SQLitePool，vtable 以裸名注册；tagengine 作为消费方自行
// 解析该持有者并按命名空间绑定到求值 ctx，避免 provider 反向依赖消费方（依赖方向倒置）。
// 同时注入通用对象缓存（s3.ObjectCache，数据源上传文件经 S3 加密存储的读写解析器）；
// S3 未配置时 store 保持 nil，sqlite 数据源创建/注册会返回「需要启用 S3」。
// newSQLiteVTabPoolsProvider creates the namespace-isolated sqlite vtable pools holder
// (vtab.Pools). Each namespace owns an independent SQLitePool and vtables are registered
// under their bare names; the tagengine consumer resolves this holder itself and binds the
// per-namespace pool to the evaluation ctx, so the provider never depends back on its
// consumer (no inverted dependency direction). It also injects a generic object cache
// (s3.ObjectCache, the S3-encrypted storage resolver for datasource-uploaded files); when
// S3 is not configured the store stays nil and sqlite datasource create/register returns
// "requires S3".
func newSQLiteVTabPoolsProvider() do.Provider[*vtab.Pools] {
	return func(i do.Injector) (*vtab.Pools, error) {
		if viper, err := InvokeViper(i); err != nil {
			return nil, errors.WithStack(err)
		} else if cfg, err := s3.ExtractConfig(viper, validator.Validate); err != nil {
			return nil, errors.WithStack(err)
		} else if client, err := InvokeS3(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			store := s3.NewObjectCache(client, *cfg)
			return vtab.NewPools(store), nil
		}
	}
}

// InjectSQLiteVTabPools 注册 sqlite vtable 池持有者 provider。
// *vtab.Pools 自身实现 do.ShutdownerWithContextAndError（Shutdown 转发
// Close 关闭全部命名空间池），故启动期经 MustInvokeSQLiteVTabPools 预解析后，
// 优雅关闭阶段由 samber/do 直接驱动其关闭，无需额外的关闭器 provider。
// InjectSQLiteVTabPools registers the sqlite vtable pools holder provider.
// *vtab.Pools itself implements do.ShutdownerWithContextAndError (Shutdown forwards
// to Close closing all namespace pools), so once eagerly resolved via
// MustInvokeSQLiteVTabPools at startup, samber/do drives its shutdown during graceful
// shutdown without an extra closer provider.
func InjectSQLiteVTabPools(i do.Injector) {
	do.Provide(i, newSQLiteVTabPoolsProvider())
}

// InvokeSQLiteVTabPools 取得 sqlite vtable 池持有者（供第三方注册数据源）。
// InvokeSQLiteVTabPools resolves the sqlite vtable pools holder (for third-party
// datasource registration).
func InvokeSQLiteVTabPools(i do.Injector) (*vtab.Pools, error) {
	return do.Invoke[*vtab.Pools](i)
}

// MustInvokeSQLiteVTabPools panic 版本取得 sqlite vtable 池持有者
// （用于启动期预解析，确保其被实例化后 Shutdown 在优雅关闭阶段被 samber/do 驱动）。
// MustInvokeSQLiteVTabPools is the panicking variant to resolve the sqlite vtable
// pools holder (eagerly resolved at startup so its Shutdown is driven during
// samber/do graceful shutdown).
func MustInvokeSQLiteVTabPools(i do.Injector) *vtab.Pools {
	return do.MustInvoke[*vtab.Pools](i)
}

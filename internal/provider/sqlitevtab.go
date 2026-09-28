package provider

import (
	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// newSQLiteVTabPoolsProvider 创建按命名空间隔离的 sqlite vtable 池持有者（vtab.Pools）。
// 每个命名空间独立持有一个 SQLitePool，vtable 以裸名注册；tagengine 作为消费方自行
// 解析该持有者并按命名空间绑定到求值 ctx，避免 provider 反向依赖消费方（依赖方向倒置）。
// 同时注入共享的通用对象缓存（s3.ObjectCache，数据源上传文件经 S3 加密存储的读写解析器）；
// S3 未配置时 store 保持 nil，sqlite 数据源创建/注册会返回「需要启用 S3」。
func newSQLiteVTabPoolsProvider() do.Provider[*vtab.Pools] {
	return func(i do.Injector) (*vtab.Pools, error) {
		if store, err := InvokeObjectCache(i); err != nil {
			return nil, errors.WithStack(err)
		} else {
			return vtab.NewPools(store), nil
		}
	}
}

// InjectSQLiteVTabPools 注册 sqlite vtable 池持有者 provider。
// *vtab.Pools 自身实现 do.ShutdownerWithContextAndError（Shutdown 转发
// Close 关闭全部命名空间池），故启动期经 MustInvokeSQLiteVTabPools 预解析后，
// 优雅关闭阶段由 samber/do 直接驱动其关闭，无需额外的关闭器 provider。
func InjectSQLiteVTabPools(i do.Injector) {
	do.Provide(i, newSQLiteVTabPoolsProvider())
}

// InvokeSQLiteVTabPools 取得 sqlite vtable 池持有者（供第三方注册数据源）。
func InvokeSQLiteVTabPools(i do.Injector) (*vtab.Pools, error) {
	return do.Invoke[*vtab.Pools](i)
}

// MustInvokeSQLiteVTabPools panic 版本取得 sqlite vtable 池持有者
// （用于启动期预解析，确保其被实例化后 Shutdown 在优雅关闭阶段被 samber/do 驱动）。
func MustInvokeSQLiteVTabPools(i do.Injector) *vtab.Pools {
	return do.MustInvoke[*vtab.Pools](i)
}

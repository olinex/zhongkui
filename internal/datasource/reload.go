// Package datasource 提供第三方数据源（通用能力）的启动重载与跨进程热同步：启动时扫描全部
// 启用数据源并注册到各自命名空间的 SQLite vtable 池，运行期经 etcd 广播变更事件，由消费进程
// 按 (Namespace, Name) 精准同步本地池。
//
// 新增数据源插件的分步清单见 docs/curated/how-to/add-datasource-plugin.md；本包注释只描述
// 重载、同步与命名空间隔离的职责边界。
package datasource

import (
	// 标准库
	"context"

	// 第三方
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab/vtab"
)

// Reload 扫描全部已启用的第三方数据源并逐个注册到其命名空间的 SQLite vtable 池。
// 逐行按 (NamespaceName, Name, Type, 明文 Config) 调用 pools.Register，注册身份为
// 该命名空间池内的裸名 {name}。单个命名空间注册失败不中断后续数据源，错误聚合并返回
// （with stack），由启动路径按既有错误处理上报。
//
// 本函数只注册当前库中启用的数据源，不清理进程重启/Down 期间已不存在的陈旧 vtable
// （存量注册身份在重新注册/注销时自然收敛）；若后续需要主动 diff 清理，可在此处扩展。
func Reload(ctx context.Context, db *gorm.DB, pools *vtab.Pools) error {
	var rows []core.Datasource
	if err := db.WithContext(ctx).Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return errors.WithStack(err)
	}
	var errs []error
	for i := range rows {
		row := &rows[i]
		if err := pools.Register(ctx, row.NamespaceName, row.Name, row.Type, row.ConfigData()); err != nil {
			errs = append(errs, errors.WithStack(errors.Newf(
				"tag datasource: reload failed for namespace %q name %q (type %s): %v",
				row.NamespaceName, row.Name, row.Type, err,
			)))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// SyncOne 按命名空间 + 裸名从数据库重读单个数据源并收敛本地池：
// 记录存在且启用 → pools.Register 覆盖注册（配置解析/建连接失败返回错误）；
// 记录不存在或停用 → pools.Unregister 幂等注销。
// 供 worker 进程收到 etcd 数据源变更事件后精准同步单个数据源，
// 避免全量重载的成本，同时保证停用/删除条目即时从池中移除。
func SyncOne(ctx context.Context, db *gorm.DB, pools *vtab.Pools, namespace, name string) error {
	var row core.Datasource
	if err := db.WithContext(ctx).Where("namespace_name = ? AND name = ?", namespace, name).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return pools.Unregister(ctx, namespace, name)
		}
		return errors.WithStack(err)
	}
	if !row.Enabled {
		return pools.Unregister(ctx, namespace, name)
	}
	return pools.Register(ctx, row.NamespaceName, row.Name, row.Type, row.ConfigData())
}

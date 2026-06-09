package core

import (
	"context"
	"sync"

	"go.uber.org/fx"
	"zhongkui.com/zhongkui/internal/depend"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Cache 是全局的命名空间缓存实例，用于存储所有命名空间名称的缓存
var (
	Cache *NamespaceCache = newNamespaceCache()
)

// NamespaceFxIn 是Fx依赖注入的输入结构体，用于提供命名空间缓存所需的依赖
type NamespaceFxIn struct {
	fx.In
	Lc fx.Lifecycle
	depend.CoreDatabaseFxIn
}

// NamespaceCache 是命名空间缓存结构体，提供线程安全的命名空间名称缓存
type NamespaceCache struct {
	inited bool           // 缓存是否已初始化
	lock   sync.RWMutex   // 读写锁，保证并发安全
	cache  map[string]any // 缓存存储，键为命名空间名称，值为nil（使用map作为集合）
}

// NewNamespaceCache 创建并返回一个新的 NamespaceCache 实例
// 用于需要独立创建缓存对象的场景（如单元测试）
func NewNamespaceCache() *NamespaceCache {
	return &NamespaceCache{
		inited: false,
		lock:   sync.RWMutex{},
		cache:  make(map[string]any, 0),
	}
}

// newNamespaceCache 创建并返回一个新的NamespaceCache实例
func newNamespaceCache() *NamespaceCache {
	return &NamespaceCache{
		inited: false,
		lock:   sync.RWMutex{},
		cache:  make(map[string]any, 0),
	}
}

// Load 从数据库加载所有命名空间名称到缓存中
// 参数：
//   - ctx: 上下文
//   - db: 数据库连接
//
// 返回值：
//   - error: 加载过程中发生的错误，如果成功则返回nil
func (n *NamespaceCache) Load(ctx context.Context, db *database.DB) error {
	if n.inited {
		return nil
	}
	n.lock.Lock()
	defer n.lock.Unlock()
	var namespaces []Namespace
	if err := db.Select("name").Find(&namespaces).Error; err != nil {
		return errors.WithStack(err)

	}
	for _, ns := range namespaces {
		n.cache[ns.Name] = nil
	}
	n.inited = true
	return nil
}

// Clear 清空缓存，将缓存状态重置为未初始化
func (n *NamespaceCache) Clear() {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.inited = false
	n.cache = make(map[string]any)
}

// All 返回缓存中所有的命名空间名称
// 返回值：
//   - []string: 命名空间名称的切片，如果缓存未初始化则返回空切片
func (n *NamespaceCache) All() []string {
	namespaces := make([]string, 0, len(n.cache))
	if !n.inited {
		return namespaces
	}
	n.lock.RLock()
	defer n.lock.RUnlock()
	for k := range n.cache {
		namespaces = append(namespaces, k)
	}
	return namespaces
}

// Add 向缓存中添加一个命名空间名称，同时将缓存标记为已初始化
// 参数：
//   - namespace: 要添加的命名空间名称
func (n *NamespaceCache) Add(namespace string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.inited = true
	n.cache[namespace] = nil
}

// set 向缓存中添加一个命名空间名称（内部方法）
// 参数：
//   - namespace: 要添加的命名空间名称
func (n *NamespaceCache) set(namespace string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.cache[namespace] = nil
}

// delete 从缓存中删除一个命名空间名称（内部方法）
// 参数：
//   - namespace: 要删除的命名空间名称
func (n *NamespaceCache) delete(namespace string) {
	n.lock.Lock()
	defer n.lock.Unlock()
	delete(n.cache, namespace)
}

// Namespace 表示命名空间模型，命名空间的名称不可修改
type Namespace struct {
	database.BaseModel        // 嵌入基础模型，包含ID、CreatedAt、UpdatedAt等字段
	Name               string `gorm:"type:VARCHAR(32)"` // 命名空间名称，唯一且不可为空，最大32字符
	Description        string `gorm:"type:VARCHAR(256)"`  // 命名空间描述，可选，最大256字符
}

// AfterCreate 在创建命名空间后触发的钩子函数，将命名空间名称添加到缓存中
// 参数：
//   - db: 数据库连接
//
// 返回值：
//   - error: 操作过程中发生的错误，如果成功则返回nil
func (n *Namespace) AfterCreate(_ *database.DB) error {
	Cache.set(n.Name)
	return nil
}

// AfterDelete 在删除命名空间后触发的钩子函数，从缓存中移除命名空间名称
// 参数：
//   - db: 数据库连接
//
// 返回值：
//   - error: 操作过程中发生的错误，如果成功则返回nil
func (n *Namespace) AfterDelete(_ *database.DB) error {
	Cache.delete(n.Name)
	return nil
}

// AfterUpdate 在更新命名空间后触发的钩子函数，检查命名空间名称是否被修改
// 由于命名空间名称不可修改，如果检测到名称被修改则返回错误
// 参数：
//   - db: 数据库连接
//
// 返回值：
//   - error: 如果名称被修改则返回错误，否则返回nil
func (n *Namespace) AfterUpdate(db *database.DB) error {
	// 使用GORM的Changed方法检查Name字段是否被修改
	// 需要检查db是否为nil，防止panic
	if db != nil && db.Statement != nil && db.Statement.Changed("Name") {
		return errors.New("namespace name cannot be modified")
	}
	return nil
}

// NamespaceExist 检查指定的命名空间名称是否存在于缓存中
// 参数：
//   - namespace: 要检查的命名空间名称
//
// 返回值：
//   - bool: 如果命名空间存在则返回true，否则返回false
func NamespaceExist(namespace string) bool {
	Cache.lock.RLock()
	defer Cache.lock.RUnlock()
	_, ok := Cache.cache[namespace]
	return ok
}

// NamespaceCacheFx 是Fx生命周期钩子函数，用于管理命名空间缓存的初始化和清理
// 参数：
//   - in: 包含Fx生命周期和数据库依赖的输入结构体
func NamespaceCacheFx(in NamespaceFxIn) {
	in.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := Cache.Load(ctx, in.CoreDB); err != nil {
				return errors.WithStack(err)
			} else {
				return nil
			}
		},
		OnStop: func(_ context.Context) error {
			Cache.Clear()
			return nil
		},
	})
}

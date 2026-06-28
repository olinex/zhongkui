package core

import (
	"context"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Cache 是全局的命名空间缓存实例，用于存储所有命名空间名称的缓存
var (
	Cache *NamespaceCache = newNamespaceCache()
)

// NamespaceCache 是命名空间缓存结构体，提供线程安全的命名空间名称缓存。
// 缓存作为性能优化层：命中时直接返回，未命中时回源数据库查询，
// 从而在无外部缓存（如 Redis）的情况下也能正确处理多实例部署场景。
type NamespaceCache struct {
	lock  sync.RWMutex   // 读写锁，保证并发安全
	cache map[string]any // 缓存存储，键为命名空间名称，值为nil（使用map作为集合）
	db    *database.DB   // 数据库连接，用于懒加载和缓存未命中时的回源查询
}

// NewNamespaceCache 创建并返回一个新的 NamespaceCache 实例
// 用于需要独立创建缓存对象的场景（如单元测试）
func NewNamespaceCache() *NamespaceCache {
	return &NamespaceCache{
		lock:  sync.RWMutex{},
		cache: make(map[string]any, 0),
	}
}

// newNamespaceCache 创建并返回一个新的NamespaceCache实例
func newNamespaceCache() *NamespaceCache {
	return &NamespaceCache{
		lock:  sync.RWMutex{},
		cache: make(map[string]any, 0),
	}
}

// SetDB 设置数据库连接引用，使缓存具备懒加载和回源查询的能力。
// 参数：
//   - db: 数据库连接
func (n *NamespaceCache) SetDB(db *database.DB) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.db = db
}

// Load 从数据库加载所有命名空间名称到缓存中，会完全替换当前缓存内容。
// 参数：
//   - ctx: 上下文
//
// 返回值：
//   - error: 加载过程中发生的错误，如果成功则返回nil
func (n *NamespaceCache) Load(ctx context.Context) error {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.loadLocked(ctx)
}

// Reload 重新从数据库加载所有命名空间名称到缓存中，是 Load 的别名。
// 参数：
//   - ctx: 上下文
//
// 返回值：
//   - error: 加载过程中发生的错误，如果成功则返回nil
func (n *NamespaceCache) Reload(ctx context.Context) error {
	return n.Load(ctx)
}

// loadLocked 在持有写锁的情况下从数据库加载命名空间（内部方法）
func (n *NamespaceCache) loadLocked(ctx context.Context) error {
	if n.db == nil {
		return errors.New("namespace cache: database not set")
	}
	var namespaces []Namespace
	if err := n.db.WithContext(ctx).Select("name").Find(&namespaces).Error; err != nil {
		return errors.WithStack(err)
	}
	n.cache = make(map[string]any, len(namespaces))
	for _, ns := range namespaces {
		n.cache[ns.Name] = nil
	}
	return nil
}

// Clear 清空缓存
func (n *NamespaceCache) Clear() {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.cache = make(map[string]any)
}

// All 返回缓存中所有的命名空间名称。
// 如果缓存为空且已设置数据库连接，会自动从数据库加载。
// 返回值：
//   - []string: 命名空间名称的切片
func (n *NamespaceCache) All(ctx context.Context) []string {
	n.lock.RLock()
	needLoad := len(n.cache) == 0 && n.db != nil
	n.lock.RUnlock()

	if needLoad {
		// 懒加载：缓存为空时自动从数据库加载
		_ = n.Load(ctx)
	}

	n.lock.RLock()
	defer n.lock.RUnlock()
	namespaces := make([]string, 0, len(n.cache))
	for k := range n.cache {
		namespaces = append(namespaces, k)
	}
	return namespaces
}

// Add 向缓存中添加一个命名空间名称
// 参数：
//   - namespace: 要添加的命名空间名称
func (n *NamespaceCache) Add(namespace string) {
	n.lock.Lock()
	defer n.lock.Unlock()
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
	Name               string `gorm:"type:VARCHAR(32);uniqueIndex"` // 命名空间名称，唯一且不可为空，最大32字符
	Description        string `gorm:"type:VARCHAR(256)"`            // 命名空间描述，可选，最大256字符
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

// NamespaceExist 检查指定的命名空间名称是否存在。
// 优先查询缓存，缓存未命中时回源数据库查询。
// 回源命中时会自动将结果写入缓存，以加速后续查询。
// 参数：
//   - namespace: 要检查的命名空间名称
//
// 返回值：
//   - bool: 如果命名空间存在则返回true，否则返回false
func NamespaceExist(namespace string) bool {
	// 快速路径：检查缓存
	Cache.lock.RLock()
	_, ok := Cache.cache[namespace]
	Cache.lock.RUnlock()
	if ok {
		return true
	}

	// 缓存未命中，回源数据库查询
	Cache.lock.RLock()
	db := Cache.db
	Cache.lock.RUnlock()

	if db == nil {
		return false
	}

	var count int64
	if err := db.Model(&Namespace{}).Where("name = ?", namespace).Count(&count).Error; err != nil {
		return false
	}
	if count > 0 {
		// 命中数据库，写入缓存
		Cache.Add(namespace)
		return true
	}
	return false
}

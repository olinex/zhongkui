package resourcepool

import (
	// 标准库
	"context"
	"io"
	"sync"

	// 第三方
	gpool "github.com/jolestar/go-commons-pool/v2"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Manager 泛型资源池管理器：按不透明 Key 惰性维护一组独立的 go-commons-pool
// 对象池，统一驱动 Lifespan 生命周期钩子，并对外提供 Acquire/Release/
// Invalidate/CloseAll。Manager 是进程级共享的，多个使用方可并发访问（线程安全）。
type Manager[T any] struct {
	// make 依据 Key 惰性构建资源对象
	make Make[T]
	// cfg 资源池配置（所有 Key 池共用）
	cfg *Config
	// pools Key -> *gpool.ObjectPool
	pools sync.Map
}

// NewManager 创建资源池管理器。cfg 为 nil 时使用 NewDefaultConfig。
func NewManager[T any](make Make[T], cfg *Config) *Manager[T] {
	if cfg == nil {
		cfg = NewDefaultConfig()
	}
	return &Manager[T]{make: make, cfg: cfg}
}

// factoryFor 构造绑定到指定 Key 的池对象工厂，将 Lifespan 钩子映射到
// go-commons-pool 的创建/校验/销毁回调。
func (m *Manager[T]) factoryFor(key Key) gpool.PooledObjectFactory {
	create := func(ctx context.Context) (any, error) {
		res, err := m.make(key)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if l, ok := any(res).(Lifespan); ok {
			if err := l.Open(ctx); err != nil {
				// 建连失败，尽力关闭以避免泄漏半初始化资源，再上抛错误
				_ = l.Close()
				return nil, errors.WithStack(err)
			}
		}
		return res, nil
	}
	validate := func(ctx context.Context, obj *gpool.PooledObject) bool {
		if l, ok := obj.Object.(Lifespan); ok {
			return l.Validate(ctx)
		}
		return true
	}
	destroy := func(ctx context.Context, obj *gpool.PooledObject) error {
		if l, ok := obj.Object.(Lifespan); ok {
			return errors.WithStack(l.Close())
		}
		if c, ok := obj.Object.(io.Closer); ok {
			return errors.WithStack(c.Close())
		}
		return nil
	}
	return gpool.NewPooledObjectFactory(create, destroy, validate, nil, nil)
}

// poolFor 惰性获取（必要时创建）属于 Key 的对象池。
func (m *Manager[T]) poolFor(key Key) *gpool.ObjectPool {
	if p, ok := m.pools.Load(key); ok {
		return p.(*gpool.ObjectPool)
	}
	created := gpool.NewObjectPool(context.Background(), m.factoryFor(key), m.cfg.toPoolConfig())
	actual, _ := m.pools.LoadOrStore(key, created)
	return actual.(*gpool.ObjectPool)
}

// Acquire 从与 key 关联的池中借出并返回一个经校验可用的资源对象。
// 同一对象在同一时刻最多被一个调用方独占；池空且允许阻塞时等待直到有对象可用
// 或 ctx 被取消。返回非 nil 错误表示借出失败（含新建时 Open 失败）。
func (m *Manager[T]) Acquire(ctx context.Context, key Key) (T, error) {
	var zero T
	obj, err := m.poolFor(key).BorrowObject(ctx)
	if err != nil {
		return zero, errors.WithStack(err)
	}
	v, ok := obj.(T)
	if !ok {
		// 类型不匹配属编程错误：失效该对象，并以 errors 携带实际类型返回，避免信息被屏蔽
		_ = m.poolFor(key).InvalidateObject(ctx, obj)
		return zero, errors.Newf("resourcepool: pool for key %q returned object of type %T, expected %T", key, obj, zero)
	}
	return v, nil
}

// Release 将资源对象归还到与 key 关联的池以便复用。
func (m *Manager[T]) Release(ctx context.Context, key Key, v T) {
	if p, ok := m.pools.Load(key); ok {
		_ = p.(*gpool.ObjectPool).ReturnObject(ctx, v)
	}
}

// Invalidate 将资源对象从池中失效并销毁（不回收复用）。
func (m *Manager[T]) Invalidate(ctx context.Context, key Key, v T) {
	if p, ok := m.pools.Load(key); ok {
		_ = p.(*gpool.ObjectPool).InvalidateObject(ctx, v)
	}
}

// Clear 销毁指定 Key 池中全部闲置对象（借出的对象不受影响）。
// 用于资源"已过期"的场景（如数据源被覆盖/注销）：闲置实例立即销毁，
// 借出的实例由调用方结合版本校验在归还时选择 Invalidate 而非 Release，
// 使下次借出必然经 Make 重建新实例。
func (m *Manager[T]) Clear(ctx context.Context, key Key) {
	if p, ok := m.pools.Load(key); ok {
		p.(*gpool.ObjectPool).Clear(ctx)
	}
}

// CloseAll 关闭全部对象池并销毁存活（闲置）对象，释放底层外部资源。
func (m *Manager[T]) CloseAll(ctx context.Context) {
	m.pools.Range(func(_, value any) bool {
		p := value.(*gpool.ObjectPool)
		p.Clear(ctx)
		p.Close(ctx)
		return true
	})
}

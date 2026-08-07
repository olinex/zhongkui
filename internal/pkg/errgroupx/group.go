// Package errgroupx 提供对 golang.org/x/sync/errgroup 的扩展封装。
// 当多个协程可能返回相同错误时，标准 errgroup 无法区分错误来源。
// 本包通过为每个 Go 调用附加标识符来解决此调试问题。
package errgroupx

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// Group 是对 errgroup.Group 的封装，为每个协程提供错误溯源能力。
// 当 f 返回非 nil 错误时，错误信息会自动包装为 "goroutine#N: <original>" 格式，
// 其中 N 为 Go 方法的调用序号（从 0 开始）。
type Group struct {
	eg      *errgroup.Group
	counter int
	mu      sync.Mutex
}

// WithContext 返回一个新的 Group 和关联的 context。
// context 会在第一个协程返回非 nil 错误时被取消。
func WithContext(ctx context.Context) (*Group, context.Context) {
	eg, ctx := errgroup.WithContext(ctx)
	return &Group{eg: eg}, ctx
}

func (g *Group) GoPrefix(prefix string, f func() error) {
	idx := g.nextIndex()
	g.eg.Go(func() error {
		if err := f(); err != nil {
			return errors.Newf("[%s#%d]: %w", prefix, idx, err)
		}
		return nil
	})
}

// Go 启动一个协程运行 f，自动分配递增的索引标识。
// 如果 f 返回错误，错误会被包装为 "goroutine#N: <original>" 格式，
// 使用 %w 保留原始错误链，因此 errors.Is / errors.As 仍然可用。
// 与 GoPrefix 的差异仅在标识格式：Go 使用匿名序号，GoPrefix 使用自定义前缀。
func (g *Group) Go(f func() error) {
	idx := g.nextIndex()
	g.eg.Go(func() error {
		if err := f(); err != nil {
			return errors.Newf("goroutine#%d: %w", idx, err)
		}
		return nil
	})
}

// GoNamed 启动一个协程运行 f，使用自定义名称标识。
// 如果 f 返回错误，错误会被包装为 "[name]: <original>" 格式。
// 适用于希望用语义化名称（而非序号）标识协程的场景。
func (g *Group) GoNamed(name string, f func() error) {
	g.eg.Go(func() error {
		if err := f(); err != nil {
			return errors.Newf("[%s]: %w", name, err)
		}
		return nil
	})
}

// SetLimit 设置并发运行的协程数量上限。
// 传递 <= 0 的值表示不限制。
func (g *Group) SetLimit(n int) {
	g.eg.SetLimit(n)
}

// Wait 等待所有协程完成并返回第一个非 nil 错误（已包装协程标识）。
func (g *Group) Wait() error {
	return g.eg.Wait()
}

// nextIndex 以线程安全的方式递增计数器并返回当前值。
func (g *Group) nextIndex() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	idx := g.counter
	g.counter++
	return idx
}

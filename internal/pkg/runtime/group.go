package runtime

import (
	"context"

	"zhongkuisec.com/zhongkui/internal/pkg/errgroupx"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Group 统一编排多个 Service 的启动与关闭生命周期。
type Group struct {
	// services 待编排生命周期的服务列表。
	services []Service
}

// NewGroup 创建服务编排组。
func NewGroup(services ...Service) *Group {
	return &Group{
		services: services,
	}
}

// Add 追加服务到编排组，可在 Run 调用前多次追加。
func (g *Group) Add(services ...Service) {
	g.services = append(g.services, services...)
}

// Run 并行启动全部服务，并统一管理生命周期。
// 每个服务在独立 goroutine 中运行；任一服务 Start 返回错误时，
// errgroup 会取消其余服务的上下文；外部 ctx 取消时全部服务退出。
// 每个服务在 Start 返回后都会调用其 Shutdown 执行优雅关闭。
//
// 返回值：首个非 nil 错误（已带 "[name]" 协程标识，errgroupx 包装），
// 全部服务正常退出时返回 nil。
func (g *Group) Run(ctx context.Context) error {
	wg, ctx := errgroupx.WithContext(ctx)
	for _, svc := range g.services {
		if n, ok := svc.(Named); ok {
			wg.GoNamed(n.Name(), func() (err error) {
				// 关闭失败同样要上报，不能被 Start 的结果吞掉；defer 保证 panic 时也会关闭。
				defer func() { err = errors.Join(err, svc.Shutdown(ctx)) }()
				return svc.Start(ctx)
			})
		} else {
			wg.Go(func() (err error) {
				defer func() { err = errors.Join(err, svc.Shutdown(ctx)) }()
				return svc.Start(ctx)
			})
		}
	}
	return wg.Wait()
}

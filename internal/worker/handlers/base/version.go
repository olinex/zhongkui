// Package base 提供分布式版本化任务处理的基础 Handler 实现。
// 包含基于 etcd 版本 CAS 机制的版本化 Handler（versionHandler）
// 和通用的批量数据流水线 Handler（BatchHandler）。
package base

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/worker/totel"
)

// VersionHandlerWrapper 版本化处理器包装接口，将普通 Handler 包装为版本感知的生命周期处理器。
// 生命周期由 versionHandler.Handle 方法编排，依次执行五个阶段：
//
//	Load      - 根据 lastVersion 从存储中加载数据
//	Do        - 执行实际的业务处理逻辑（通过注入 scope 传递数据）
//	Next      - 计算下一个版本号 nextVersion
//	Store     - 将处理后的数据持久化到存储
//	Clear     - 清理旧版本数据，防止数据无限增长
type VersionHandlerWrapper[T any] interface {
	pipeline.Handler
	Next(ctx context.Context, name, lastVersion string) (nextVersion string, err error)
	Load(ctx context.Context, name, lastVersion string) (*T, error)
	Store(ctx context.Context, name, nextVersion string, data *T) error
	Clear(ctx context.Context, name, lastVersion string) error
}

type versionHandler[T any] struct {
	task    *pipeline.Task
	wrapper VersionHandlerWrapper[T]
}

func (w *versionHandler[T]) Name() string {
	return w.task.Type()
}

// Handle 执行版本化任务处理流程，实现 etcd.Version.Commit 所需的 Handler 接口。
// 流程：创建 DI scope → Load 加载数据 → 注入 scope → Do 执行业务逻辑 →
//
//	Next 计算下一版本 → Store 持久化数据 → Clear 清理旧版本。
func (w *versionHandler[T]) Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (nextVersion string, err error) {
	name := w.Name()
	ctx, span := totel.StartVersionTaskSpan(ctx, name, lastVersion)
	defer totel.EndSpan(span, &err)
	defer func() {
		if err != nil {
			totel.RecordTaskError(ctx, name)
		} else {
			totel.RecordTask(ctx, name)
		}
	}()

	// 获取注入器
	scope := provider.GetScope(ctx)
	defer scope.Shutdown()
	// 加载数据
	innerCtx, loadSpan := totel.StartVersionLoadSpan(ctx, name, lastVersion)
	data, lErr := w.wrapper.Load(innerCtx, name, lastVersion)
	totel.EndSpan(loadSpan, &lErr)
	if lErr != nil {
		return "", errors.WithStack(lErr)
	}
	if data == nil {
		return "", errors.New("data is nil")
	}
	// 动态生成注入器
	provider := func(i do.Injector) (*T, error) {
		return data, nil
	}
	// 注入数据
	do.Provide(scope, provider)
	// 执行任务
	err = w.wrapper.Do(ctx, w.task)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 计算下一个版本
	innerCtx, nextSpan := totel.StartVersionNextSpan(ctx, name, lastVersion)
	nextVersion, err = w.wrapper.Next(innerCtx, name, lastVersion)
	totel.EndSpan(nextSpan, &err)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 存储数据
	innerCtx, storeSpan := totel.StartVersionStoreSpan(ctx, name, nextVersion)
	err = w.wrapper.Store(innerCtx, name, nextVersion, data)
	totel.EndSpan(storeSpan, &err)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 清除旧版本数据
	innerCtx, clearSpan := totel.StartVersionClearSpan(ctx, name, lastVersion)
	err = w.wrapper.Clear(innerCtx, name, lastVersion)
	totel.EndSpan(clearSpan, &err)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return nextVersion, nil
}

// VersionHandler 分布式版本化任务处理器，使用 etcd Version 的 CAS（compare-and-swap）机制
// 保证多实例间任务执行的互斥与幂等。每个 Handle 周期从存储加载数据版本、执行处理、
// 计算下一版本并持久化，通过 etcd 的原子 CAS 提交确保分布式一致性。
type VersionHandler[T any] struct {
	wrapper VersionHandlerWrapper[T]
}

// NewVersionHandler 创建 VersionHandler 实例并初始化 OpenTelemetry 指标。
// wrapper 必须实现 VersionHandlerWrapper[T] 接口，提供版本感知的 Load/Do/Next/Store/Clear 生命周期方法。
func NewVersionHandler[T any](wrapper VersionHandlerWrapper[T]) *VersionHandler[T] {
	totel.InitMetrics()
	return &VersionHandler[T]{
		wrapper: wrapper,
	}
}

// Do 执行版本化任务的入口方法。
// 流程：从 DI scope 获取 etcd client 和 logger → 创建内部 versionHandler →
//
//	通过 etcd.Version.Commit 提交，利用 etcd CAS 机制保证分布式互斥。
func (w *VersionHandler[T]) Do(ctx context.Context, task *pipeline.Task) error {
	scope := provider.GetScope(ctx)
	defer scope.Shutdown()
	client, err := provider.InvokeEtcdClient(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	logger, err := provider.InvokePipelineLogger(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	handler := &versionHandler[T]{
		task:    task,
		wrapper: w.wrapper,
	}
	version := etcd.NewVersion(client, logger, handler)
	// 提交版本
	err = version.Commit(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

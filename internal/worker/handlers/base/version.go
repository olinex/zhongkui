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

type VersionHandler[T any] struct {
	wrapper VersionHandlerWrapper[T]
}

func NewVersionHandler[T any](wrapper VersionHandlerWrapper[T]) *VersionHandler[T] {
	totel.InitMetrics()
	return &VersionHandler[T]{
		wrapper: wrapper,
	}
}

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

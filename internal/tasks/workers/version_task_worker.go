package workers

import (
	"context"

	"github.com/samber/do/v2"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/provider"
)

type VersionTaskWrapper[T any] interface {
	pipeline.TaskWorker
	Next(ctx context.Context, name, lastVersion string) (nextVersion string, err error)
	Load(ctx context.Context, name, lastVersion string) (*T, error)
	Store(ctx context.Context, name, nextVersion string, data *T) error
	Clear(ctx context.Context, name, lastVersion string) error
}

type versionTaskHandler[T any] struct {
	task    *pipeline.Task
	wrapper VersionTaskWrapper[T]
}

func (w *versionTaskHandler[T]) Name() string {
	return w.task.Type()
}

func (w *versionTaskHandler[T]) Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (string, error) {
	name := w.Name()
	// 获取注入器
	scope := pipeline.GetScope(ctx)
	defer scope.Shutdown()
	// 加载数据
	data, err := w.wrapper.Load(ctx, name, lastVersion)
	if err != nil {
		return "", errors.WithStack(err)
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
	nextVersion, err := w.wrapper.Next(ctx, name, lastVersion)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 存储数据
	err = w.wrapper.Store(ctx, name, nextVersion, data)
	if err != nil {
		return "", errors.WithStack(err)
	}
	// 清除旧版本数据
	err = w.wrapper.Clear(ctx, name, lastVersion)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return nextVersion, nil
}

type VersionTaskWorker[T any] struct {
	wrapper VersionTaskWrapper[T]
}

func NewVersionTaskWorker[T any](wrapper VersionTaskWrapper[T]) *VersionTaskWorker[T] {
	return &VersionTaskWorker[T]{
		wrapper: wrapper,
	}
}

func (w *VersionTaskWorker[T]) Do(ctx context.Context, task *pipeline.Task) error {
	scope := pipeline.GetScope(ctx)
	defer scope.Shutdown()
	client, err := provider.InvokeEtcdClient(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	logger, err := provider.InvokePipelineLogger(scope)
	if err != nil {
		return errors.WithStack(err)
	}
	handler := &versionTaskHandler[T]{
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

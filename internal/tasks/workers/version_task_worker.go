package workers

import (
	"context"

	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

type TaskVersionHandler interface {
	Name() string
	Handle(ctx context.Context, logger *logging.Logger, task *pipeline.Task, lastVersion string) (nextVersion string, err error)
}

type taskVersionWrapper struct {
	task    *pipeline.Task
	handler TaskVersionHandler
}

func (w *taskVersionWrapper) Name() string {
	return w.handler.Name()
}

func (w *taskVersionWrapper) Handle(ctx context.Context, logger *logging.Logger, lastVersion string) (nextVersion string, err error) {
	return w.handler.Handle(ctx, logger, w.task, lastVersion)
}

type VersionTaskWorker struct {
	client  *etcd.EtcdClient
	logger  *logging.Logger
	handler TaskVersionHandler
}

func NewVersionTaskWorker(client *etcd.EtcdClient, logger *logging.Logger, handler TaskVersionHandler) *VersionTaskWorker {
	return &VersionTaskWorker{
		client:  client,
		logger:  logger,
		handler: handler,
	}
}

func (w *VersionTaskWorker) Do(ctx context.Context, task *pipeline.Task) error {
	wrapper := taskVersionWrapper{
		task:    task,
		handler: w.handler,
	}
	version := etcd.NewVersion(w.client, w.logger, &wrapper)
	return version.Commit(ctx)
}

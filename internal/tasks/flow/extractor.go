package flow

import (
	"context"

	"github.com/reactivex/rxgo/v2"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
	"zhongkui.com/zhongkui/internal/tasks/workers"
)

func ExtractHttpTransportFromTCPIPv4Model(ctx context.Context, task *pipeline.Task, channel chan<- rxgo.Item) error {
	workers.GetFxIn(ctx)
	return nil
}

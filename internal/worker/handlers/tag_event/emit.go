package tag_event

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/provider"
	"zhongkui.com/zhongkui/internal/tagengine"
)

const (
	// TagEventEmitTask 标签事件处理任务类型，检测命中后由检测逻辑投递、worker 消费。
	TagEventEmitTask = "tag:event:emit"
)

// 编译期断言：确保 EventEmitHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventEmitHandler)(nil)

// EventEmitHandler 标签事件处理处理器，消费 tag:event:emit 任务。
// 本阶段仅解析载荷并记录日志，作为后续事件落库/通知的扩展点。
type EventEmitHandler struct {
	logger *logging.Logger // 日志记录器
}

// NewEventEmitHandler 创建标签事件处理处理器。
func NewEventEmitHandler(i do.Injector) *EventEmitHandler {
	logger := provider.MustInvokeDatabaseLogger(i)
	return &EventEmitHandler{logger: logger}
}

// Do 消费一条标签事件：解析载荷并记录结构化日志；载荷非法返回错误触发重试。
func (h *EventEmitHandler) Do(ctx context.Context, task etl.Task) error {
	var ev tagengine.TagEvent
	if err := json.Unmarshal(task.Payload(), &ev); err != nil {
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Infof("tag event emit: rule=%s namespace=%s matchAll=%v hits=%d",
		ev.RuleName, ev.Namespace, ev.MatchAll, len(ev.Hits))
	return nil
}

// NewTagEventEmitTask 将标签事件编码为 tag:event:emit 异步任务。
func NewTagEventEmitTask(ev *tagengine.TagEvent) (*asynq.Task, error) {
	if ev == nil {
		return nil, errors.New("tag event is nil")
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(TagEventEmitTask, payload), nil
}

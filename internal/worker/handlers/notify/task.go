// Package notify 提供通用通知发送的 asynq 任务处理器。
// SendGroupHandler 消费标签事件等事件源投递的 notify:send:group 任务：解析
// SendGroupInput 载荷后，调用 internal/notify 的 Service.ResolveGroupTargets 将通知组
// 分解为逐目标直投入参，再以 SendTargetHandler 的 notify:send:target 任务逐目标扇出投递。
// SendTargetHandler 消费 notify:send:target 任务：解析 SendTargetInput 载荷后，调用
// internal/notify 的 Service 不经通知组直接投递单个目标（SendToTarget）。
//
// Package notify provides the asynq handlers for generic notification sending.
// SendGroupHandler consumes notify:send:group tasks enqueued by event sources such as
// tag events: after parsing the SendGroupInput payload it calls the internal/notify
// Service.ResolveGroupTargets to expand the group into per-target direct-send inputs,
// then fans out one notify:send:target task per target, handled by SendTargetHandler.
// SendTargetHandler consumes notify:send:target tasks: after parsing the
// SendTargetInput payload it calls the internal/notify Service to deliver directly to
// a single target without a group.
package notify

import (
	// stdlib
	"context"
	"encoding/json"

	// third-party
	"github.com/hibiken/asynq"
	"github.com/samber/do/v2"

	// internal
	notifybase "zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/validator"
	"zhongkui.com/zhongkui/internal/provider"
)

const (
	// SendGroupTask 通用通知组发送任务类型，载荷为 SendGroupInput（命名空间 + 通知组 +
	// 事件引用 + 消息）。与事件类型解耦：标签事件及其他事件源均可投递该任务，由 worker
	// 消费后先解析为逐目标直投再扇出投递，最终经渠道基座发送。
	// SendGroupTask is the generic notification group-send task type carrying a
	// SendGroupInput payload (namespace + group + event reference + message). It is
	// decoupled from event types: tag events and other event sources can enqueue it, and
	// the worker expands it into per-target direct sends and fans them out via the
	// channel base.
	SendGroupTask = "notify:send:group"

	// SendTargetTask 单目标直投任务类型，载荷为 SendTargetInput（命名空间 + 目标 ID + 事件引用 + 消息）。
	// 不经过通知组，直接投递单个目标，由 worker 消费后经渠道基座发送。
	// SendTargetTask is the direct single-target send task type carrying a SendTargetInput
	// payload (namespace + target ID + event reference + message). It skips notification
	// groups, delivers directly to a single target, and the worker sends via the channel base.
	SendTargetTask = "notify:send:target"
)

// taskEnqueuer 任务投递客户端接口，定义在消费侧并与 tag_event 包解耦（不能 import 该包），
// 便于单元测试注入替代实现。
// taskEnqueuer is the task enqueuing client interface, defined on the consumer side and
// kept decoupled from the tag_event package (which cannot be imported here), so tests can
// inject a substitute implementation.
type taskEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// 编译期断言：确保 SendGroupHandler 实现了 etl.Handler 接口。
// Compile-time assertion that SendGroupHandler implements etl.Handler.
var _ etl.Handler = (*SendGroupHandler)(nil)

// 编译期断言：确保 SendTargetHandler 实现了 etl.Handler 接口。
// Compile-time assertion that SendTargetHandler implements etl.Handler.
var _ etl.Handler = (*SendTargetHandler)(nil)

// SendGroupHandler 通用通知组发送处理器，消费 notify:send:group 任务。
// 解析载荷后调用 Service.ResolveGroupTargets 将通知组分解为逐目标直投入参，再为每个
// 目标投递一条 notify:send:target 任务（由 SendTargetHandler 消费完成实际发送与记录写入）；
// 载荷非法返回错误触发重试。
// SendGroupHandler is the generic notification group-send handler consuming
// notify:send:group tasks. It parses the payload and calls Service.ResolveGroupTargets
// to expand the group into per-target direct-send inputs, then enqueues one
// notify:send:target task per target (consumed by SendTargetHandler, which performs the
// actual delivery and record writes); an invalid payload returns an error to trigger a
// retry.
type SendGroupHandler struct {
	service *notifybase.Service
	client  taskEnqueuer
	logger  *logging.Logger
}

// NewSendGroupHandler 从 do.Injector 解析依赖并创建通用通知组发送处理器。
// NewSendGroupHandler resolves dependencies from the do.Injector and creates the handler.
func NewSendGroupHandler(i do.Injector) *SendGroupHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	client := provider.MustInvokePipelineClient(i)
	pool := provider.MustInvokeMessagePool(i)
	return &SendGroupHandler{service: notifybase.NewService(coreDB, logger, pool), client: client, logger: logger}
}

// Do 消费一条 notify:send:group 任务：解析 SendGroupInput 载荷，解析通知组并扇出为
// 逐目标直投任务；载荷非法返回错误触发重试。任一目标任务投递失败都会累计并返回非 nil
// 错误以触发重试，目标级幂等可在重试时去重。
// Do consumes one notify:send:group task: it parses the SendGroupInput payload, resolves
// the group and fans out one direct-send task per target. A failure to enqueue any target
// task is accumulated into a non-nil error to trigger a retry; target-level idempotency
// deduplicates on retry.
func (h *SendGroupHandler) Do(ctx context.Context, task etl.Task) error {
	var input notifybase.SendGroupInput
	if err := json.Unmarshal(task.Payload(), &input); err != nil {
		return errors.WithStack(err)
	}
	if err := validateSendGroupInput(input); err != nil {
		return err
	}
	targets, err := h.service.ResolveGroupTargets(ctx, input)
	if err != nil {
		h.logger.Ctx(ctx).Errorf("notify: resolve group targets failed (namespace=%s groupID=%v event=%s/%d): %v",
			input.Namespace, input.GroupID, input.EventType, input.EventID, err)
		return err
	}
	h.logger.Ctx(ctx).Infof("notify: fan out %d send:target tasks (namespace=%s groupID=%v event=%s/%d)",
		len(targets), input.Namespace, input.GroupID, input.EventType, input.EventID)
	var enqueueErr error
	for _, target := range targets {
		t, err := NewSendTargetTask(target)
		if err != nil {
			enqueueErr = errors.Join(enqueueErr, err)
			continue
		}
		if _, err := h.client.EnqueueContext(ctx, t, asynq.MaxRetry(3)); err != nil {
			enqueueErr = errors.Join(enqueueErr, err)
		}
	}
	return enqueueErr
}

// NewSendGroupTask 将通知组发送请求编码为 notify:send:group 异步任务；必填字段缺失返回错误。
// NewSendGroupTask encodes a group notification send request into a notify:send:group
// async task; missing required fields return an error.
func NewSendGroupTask(in notifybase.SendGroupInput) (*asynq.Task, error) {
	if err := validateSendGroupInput(in); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(&in)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(SendGroupTask, payload), nil
}

// validateSendGroupInput 校验通知组发送载荷的必填字段：命名空间、事件类型与消息模板键。
// 校验规则声明在 SendGroupInput / Message 结构体的 binding 标签上，经全局 validator 引擎
// 统一执行（validator.Validate.Struct），错误为 validator.ValidationErrors。
// validateSendGroupInput validates the required fields of a group-send payload: namespace,
// event type and the message template key. The rules are declared as binding tags on the
// SendGroupInput / Message structs and enforced by the global validator engine via
// validator.Validate.Struct, returning a validator.ValidationErrors on failure.
func validateSendGroupInput(in notifybase.SendGroupInput) error {
	return errors.WithStack(validator.Validate.Struct(&in))
}

// SendTargetHandler 单目标直投处理器，消费 notify:send:target 任务。
// 解析载荷后调用 Service.SendToTarget 不经通知组直接投递单个目标；
// 载荷非法返回错误触发重试。
// SendTargetHandler is the direct single-target send handler consuming
// notify:send:target tasks. It parses the payload and calls Service.SendToTarget to
// deliver directly to a single target without a group; an invalid payload returns an
// error to trigger a retry.
type SendTargetHandler struct {
	service *notifybase.Service
	logger  *logging.Logger
}

// NewSendTargetHandler 从 do.Injector 解析依赖并创建单目标直投处理器。
// NewSendTargetHandler resolves dependencies from the do.Injector and creates the handler.
func NewSendTargetHandler(i do.Injector) *SendTargetHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	pool := provider.MustInvokeMessagePool(i)
	return &SendTargetHandler{service: notifybase.NewService(coreDB, logger, pool), logger: logger}
}

// Do 消费一条 notify:send:target 任务：解析 SendTargetInput 载荷并执行直投；载荷非法返回错误触发重试。
// Do consumes one notify:send:target task: it parses the SendTargetInput payload and
// delivers directly; an invalid payload returns an error to trigger a retry.
func (h *SendTargetHandler) Do(ctx context.Context, task etl.Task) error {
	var input notifybase.SendTargetInput
	if err := json.Unmarshal(task.Payload(), &input); err != nil {
		return errors.WithStack(err)
	}
	if err := validateSendTargetInput(input); err != nil {
		return err
	}
	if err := h.service.SendToTarget(ctx, input); err != nil {
		h.logger.Ctx(ctx).Errorf("notify: send-to-target failed (namespace=%s targetID=%d event=%s/%d): %v",
			input.Namespace, input.TargetID, input.EventType, input.EventID, err)
		return err
	}
	return nil
}

// NewSendTargetTask 将单目标直投请求编码为 notify:send:target 异步任务；必填字段缺失返回错误。
// NewSendTargetTask encodes a direct single-target send request into a
// notify:send:target async task; missing required fields return an error.
func NewSendTargetTask(in notifybase.SendTargetInput) (*asynq.Task, error) {
	if err := validateSendTargetInput(in); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(&in)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return asynq.NewTask(SendTargetTask, payload), nil
}

// validateSendTargetInput 校验单目标直投载荷的必填字段：命名空间、事件类型、消息模板键与目标 ID。
// 校验规则声明在 SendTargetInput / Message 结构体的 binding 标签上，经全局 validator 引擎
// 统一执行（validator.Validate.Struct），错误为 validator.ValidationErrors。
// validateSendTargetInput validates the required fields of a direct-send payload:
// namespace, event type, the message template key and the target ID. The rules are
// declared as binding tags on the SendTargetInput / Message structs and enforced by
// the global validator engine via validator.Validate.Struct, returning a
// validator.ValidationErrors on failure.
func validateSendTargetInput(in notifybase.SendTargetInput) error {
	return errors.WithStack(validator.Validate.Struct(&in))
}

package tagengine

import (
	// stdlib
	"context"
	"encoding/json"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
)

// ruleChangeKey 规则变更广播在 etcd 中的全局键。
// 写路径在 DB 事务提交后向该键写入规则变更事件负载，各进程的引擎
// watch 该键并失效对应分组的编译缓存，保证多实例部署下所有使用方
// 收敛到同一套规则。
var ruleChangeKey = etcd.WithPrefix("tagengine/") + "rule-change"

// RuleChangeEvent 规则变更事件负载。
// 记录本次写路径影响的分组（Namespace + Model），供引擎精准失效
// 对应分组缓存，避免全量失效的高成本；解析失败时回退全量失效保证不遗漏。
type RuleChangeEvent struct {
	Namespace string `json:"namespace"`
	Model     string `json:"model"`
}

// BroadcastChange 广播规则变更事件。
// 每次规则写路径（增删改/启停/回滚）在 DB 事务提交后调用，
// 通知所有进程的引擎失效对应分组缓存并按 DB 最新规则懒重编译。
// 发布带超时上限，失败返回错误但不影响调用方；未绑定 etcd 客户端时
// 返回 etcd.ErrChangeUnavailable。
// 注意：DB 提交先于 etcd 发布，保证收到通知的引擎读到的必是新规则。
func (e *Engine) BroadcastChange(ctx context.Context, namespace, model string) error {
	return etcd.BroadcastChange(ctx, e.client, ruleChangeKey, &RuleChangeEvent{Namespace: namespace, Model: model})
}

// ruleChangeHandler 规则变更事件处理器，实现 etcd.ChangeHandler。
// 收到负载后解析规则变更事件并精准失效对应分组缓存；
// 负载无法解析时回退全量失效，保证不遗漏任何变更。
type ruleChangeHandler struct {
	e *Engine
}

// HandleChange 处理单条规则变更事件负载。
// 解析失败仅记录告警并回退全量失效，不中断后续事件处理。
func (h *ruleChangeHandler) HandleChange(_ context.Context, value []byte) error {
	event, err := parseRuleChangeEvent(value)
	if err != nil {
		h.e.logger.Warnf("tagengine rule change event unparsable, fallback to invalidate all: %v", err)
		h.e.InvalidateAll()
		return nil
	}
	h.e.Invalidate(event.Namespace, event.Model)
	return nil
}

// Start 阻塞运行规则变更监听，直到 ctx 取消。实现 runtime.Service 生命周期。
// 未构造 watcher 时记录跳过信息并直接返回；否则委托通用 etcd watcher 监听
// 规则变更，watch 中断自动重连，事件处理由 ruleChangeHandler 完成。
func (e *Engine) Start(ctx context.Context) error {
	if e.watcher == nil {
		e.logger.Ctx(ctx).Info("tagengine rule change watch skipped: no etcd client")
		return nil
	}
	return e.watcher.Start(ctx)
}

// Shutdown 停止规则变更监听。
// 由 runtime 编排器在 Start 返回后调用，实际停止依赖 Start 的 ctx 取消。
func (e *Engine) Shutdown(ctx context.Context) error {
	if e.watcher == nil {
		return nil
	}
	return e.watcher.Shutdown(ctx)
}

// parseRuleChangeEvent 解析规则变更事件负载。
// 负载不是合法 JSON 或分组信息缺失时返回错误，调用方应回退全量失效。
func parseRuleChangeEvent(value []byte) (*RuleChangeEvent, error) {
	event := &RuleChangeEvent{}
	if err := json.Unmarshal(value, event); err != nil {
		return nil, errors.WithStack(err)
	}
	if event.Model == "" {
		return nil, errors.New("tagrule rule change event misses group info")
	}
	return event, nil
}

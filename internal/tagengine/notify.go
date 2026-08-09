package tagengine

import (
	"context"
	"encoding/json"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
)

const (
	// publishTimeout 单次规则变更发布的超时上限。
	// etcd 端点不可达（如测试环境、etcd 故障）且未配置 DialTimeout 时，
	// Put 的 gRPC 重试会无限等待；加超时保证发布快速失败，
	// 由写路径告警降级而非阻塞响应。
	publishTimeout = 3 * time.Second
	// watchRetryInterval 规则变更 watch 中断后的重连间隔。
	watchRetryInterval = 3 * time.Second
)

// ruleChangeKey 规则变更广播在 etcd 中的全局键。
// 写路径在 DB 事务提交后向该键写入规则变更事件负载，各进程的引擎
// watch 该键并失效对应分组的编译缓存，保证多实例部署下所有使用方
// 收敛到同一套规则。
var ruleChangeKey = etcd.WithPrefix("tagengine/") + "rule-change"

// RuleChangeEvent 规则变更事件负载。
// 记录本次写路径影响的分组（Namespace + TargetType + TagType），供引擎精准失效
// 对应分组缓存，避免全量失效的高成本；解析失败时回退全量失效保证不遗漏。
type RuleChangeEvent struct {
	Namespace  string `json:"namespace"`
	TargetType string `json:"targetType"`
	TagType    string `json:"tagType"`
}

// ErrRuleChangeUnavailable 规则变更广播不可用的错误。
// 引擎未绑定 etcd 客户端（纯本地/测试场景）时广播无法发出，
// 写路径收到该错误仅告警降级，不影响已提交的规则变更。
var ErrRuleChangeUnavailable = errors.New("tagengine rule change broadcast unavailable: no etcd client")

// BroadcastChange 广播规则变更事件。
// 每次规则写路径（增删改/启停/回滚）在 DB 事务提交后调用，
// 通知所有进程的引擎失效对应分组缓存并按 DB 最新规则懒重编译。
// 发布带超时上限，失败返回错误但不影响调用方；未绑定 etcd 客户端时
// 返回 ErrRuleChangeUnavailable。
// 注意：DB 提交先于 etcd 发布，保证收到通知的引擎读到的必是新规则。
func (e *Engine) BroadcastChange(ctx context.Context, namespace, targetType, tagType string) error {
	if e.client == nil {
		return ErrRuleChangeUnavailable
	}
	payload, err := json.Marshal(&RuleChangeEvent{Namespace: namespace, TargetType: targetType, TagType: tagType})
	if err != nil {
		return errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if _, err := e.client.Put(ctx, ruleChangeKey, string(payload)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Start 阻塞运行规则变更监听，直到 ctx 取消。实现 runtime.Service 生命周期。
// 收到变更事件后失效对应分组的编译缓存，下次评估按 DB 最新规则懒重编译；
// 事件负载无法解析时回退全量失效，保证不遗漏任何变更。
// watch 中断自动重连；未绑定 etcd 客户端时直接返回（无监听能力）。
func (e *Engine) Start(ctx context.Context) error {
	if e.client == nil {
		e.logger.Ctx(ctx).Info("tagengine rule change watch skipped: no etcd client")
		return nil
	}
	e.logger.Ctx(ctx).Info("tagengine rule change watcher started")
	for {
		if err := e.watchLoop(ctx); err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			e.logger.Ctx(ctx).Warnf("tagengine rule change watch interrupted, retrying in %s: %v", watchRetryInterval, err)
			timer := time.NewTimer(watchRetryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
}

// Shutdown 停止规则变更监听。
// 由 runtime 编排器在 Start 返回后调用，实际停止依赖 Start 的 ctx 取消。
func (e *Engine) Shutdown(_ context.Context) error {
	return nil
}

// watchLoop 执行一次 watch 会话，处理变更事件。
// 会话被取消或中断时返回错误，由 Start 决定重连或退出。
func (e *Engine) watchLoop(ctx context.Context) error {
	rch := e.client.Watch(ctx, ruleChangeKey)
	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-rch:
			if !ok {
				return errors.New("tagengine rule change watch channel closed")
			}
			if err := e.handleResponse(resp); err != nil {
				return err
			}
		}
	}
}

// handleResponse 处理单次 watch 响应：校验错误并失效对应分组的引擎缓存。
// 无事件（如创建/进度通知）时直接返回；事件负载无法解析时回退全量失效。
func (e *Engine) handleResponse(resp clientv3.WatchResponse) error {
	if err := resp.Err(); err != nil {
		return errors.WithStack(err)
	}
	if len(resp.Events) == 0 {
		return nil
	}
	for _, ev := range resp.Events {
		event, err := parseRuleChangeEvent(ev.Kv.Value)
		if err != nil {
			e.logger.Warnf("tagengine rule change event unparsable, fallback to invalidate all: %v", err)
			e.InvalidateAll()
			return nil
		}
		e.Invalidate(event.Namespace, event.TargetType, event.TagType)
	}
	e.logger.Info("tagengine rules changed, invalidating group caches")
	return nil
}

// parseRuleChangeEvent 解析规则变更事件负载。
// 负载不是合法 JSON 或分组信息缺失时返回错误，调用方应回退全量失效。
func parseRuleChangeEvent(value []byte) (*RuleChangeEvent, error) {
	event := &RuleChangeEvent{}
	if err := json.Unmarshal(value, event); err != nil {
		return nil, errors.WithStack(err)
	}
	if event.TargetType == "" || event.TagType == "" {
		return nil, errors.New("tagrule rule change event misses group info")
	}
	return event, nil
}

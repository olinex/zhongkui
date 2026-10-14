// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
//
// 此外，该包还基于 etcd concurrency 包实现了分布式领导者选举机制，
// 支持多实例部署场景下的主备切换和高可用调度。
// 同时提供通用的 etcd 变更广播（BroadcastChange）与变更监听（Watcher）能力，
// 供 tagengine、datasource 等业务包复用，实现多进程间的配置热同步。
package etcd

import (
	// 标准库
	"context"
	"time"

	// 第三方
	clientv3 "go.etcd.io/etcd/client/v3"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

const (
	// watchRetryInterval 变更 watch 中断后的默认重连间隔。
	// 可通过 WithWatchRetryInterval 选项覆盖。
	watchRetryInterval = 3 * time.Second
)

// ChangeHandler 变更事件处理器接口。
// 消费方负责解析负载并执行同步/失效逻辑；解析失败的回退策略由消费方自行决定。
type ChangeHandler interface {
	// HandleChange 处理单个变更事件负载。
	// 返回错误时由 Watcher 记录告警，不中断后续事件处理。
	HandleChange(ctx context.Context, value []byte) error
}

// ChangeHandlerFunc 函数适配器，将普通函数适配为 ChangeHandler。
// 便于消费方以函数字面量形式注册变更处理逻辑。
type ChangeHandlerFunc func(ctx context.Context, value []byte) error

// HandleChange 调用适配的函数，实现 ChangeHandler 接口。
func (f ChangeHandlerFunc) HandleChange(ctx context.Context, value []byte) error {
	return f(ctx, value)
}

// watchOptions Watcher 的选项集合。
type watchOptions struct {
	// retryInterval watch 中断后的重连间隔。
	retryInterval time.Duration
}

// WatchOption 定义 Watcher 的可选参数，以函数式选项模式配置监听器。
type WatchOption func(*watchOptions)

// WithWatchRetryInterval 覆盖 watch 中断后的重连间隔（默认 watchRetryInterval）。
// 测试场景可传入极小间隔（如 time.Millisecond）以加速重连验证。
func WithWatchRetryInterval(interval time.Duration) WatchOption {
	return func(o *watchOptions) {
		o.retryInterval = interval
	}
}

// Watcher 通用的 etcd 变更监听器，实现 runtime.Service 生命周期。
// 监听指定键的变更事件，逐事件回调 handler 执行同步/失效逻辑；
// watch 中断自动重连；未绑定 etcd 客户端时直接返回（无监听能力）。
type Watcher struct {
	// client etcd 客户端实例，nil 时监听被跳过。
	client *EtcdClient
	// key 监听的 etcd 键。
	key string
	// handler 变更事件处理器。
	handler ChangeHandler
	// logger 日志记录器，记录监听状态与事件处理告警。
	logger *logging.Logger
	// opts 监听选项集合（重连间隔等可调参数）。
	opts watchOptions
	// watch 可注入的 watch 函数，默认调用 client.Watch，供白盒测试模拟断线重连。
	watch func(ctx context.Context, key string) clientv3.WatchChan
}

// NewWatcher 创建变更监听器。
// 应用选项后 watch 字段默认为 client.Watch 的包装，测试可手工覆盖为 fake 函数。
//
// 参数:
//   - client: etcd 客户端实例，可为 nil（此时 Start 直接返回，无监听能力）
//   - key: 监听的 etcd 键
//   - handler: 变更事件处理器
//   - logger: 日志记录器
//   - opts: 可选的监听选项（如重连间隔覆盖）
//
// 返回值:
//   - *Watcher: 初始化完成的监听器实例
func NewWatcher(client *EtcdClient, key string, handler ChangeHandler, logger *logging.Logger, opts ...WatchOption) *Watcher {
	w := &Watcher{
		client:  client,
		key:     key,
		handler: handler,
		logger:  logger,
		opts:    watchOptions{retryInterval: watchRetryInterval},
	}
	for _, opt := range opts {
		opt(&w.opts)
	}
	w.watch = func(ctx context.Context, key string) clientv3.WatchChan {
		return client.Watch(ctx, key)
	}
	return w
}

// Start 阻塞运行变更监听，直到 ctx 取消。实现 runtime.Service 生命周期。
// 收到变更事件后逐事件回调 handler；watch 中断自动重连；
// 未绑定 etcd 客户端时直接返回（无监听能力）。
func (w *Watcher) Start(ctx context.Context) error {
	if w.client == nil {
		w.logger.Ctx(ctx).Info("etcd change watcher skipped: no etcd client")
		return nil
	}
	for {
		if err := w.watchLoop(ctx); err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			w.logger.Ctx(ctx).Warnf("etcd change watcher interrupted, retrying in %s: %v", w.opts.retryInterval, err)
			timer := time.NewTimer(w.opts.retryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		} else {
			// watchLoop 仅在 ctx 取消时返回 nil，此时直接退出，避免空转重连。
			return nil
		}
	}
}

// Shutdown 停止变更监听。
// 由 runtime 编排器在 Start 返回后调用，实际停止依赖 Start 的 ctx 取消。
func (w *Watcher) Shutdown(_ context.Context) error {
	return nil
}

// watchLoop 执行一次 watch 会话，处理变更事件。
// 会话被取消时返回 nil；通道关闭或响应错误时返回错误，由 Start 决定重连或退出。
func (w *Watcher) watchLoop(ctx context.Context) error {
	rch := w.watch(ctx, w.key)
	for {
		select {
		case <-ctx.Done():
			return nil
		case resp, ok := <-rch:
			if !ok {
				return errors.New("etcd change watch channel closed")
			}
			if err := w.handleResponse(ctx, resp); err != nil {
				return err
			}
		}
	}
}

// handleResponse 处理单次 watch 响应：校验错误并逐事件回调处理器。
// 无事件（如创建/进度通知）时直接返回；单个事件回调失败仅记录告警，
// 不中断后续事件处理。
func (w *Watcher) handleResponse(ctx context.Context, resp clientv3.WatchResponse) error {
	if err := resp.Err(); err != nil {
		return errors.WithStack(err)
	}
	if len(resp.Events) == 0 {
		return nil
	}
	for _, ev := range resp.Events {
		if err := w.handler.HandleChange(ctx, ev.Kv.Value); err != nil {
			w.logger.Warnf("etcd change handler failed: %v", err)
		}
	}
	return nil
}

package ai

import (
	// stdlib
	"context"
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// ChangeKey 是 LLM 配置变更广播在 etcd 中的全局键。
// 管理端写路径（模型注册、档位绑定、参数覆盖）在 DB 提交后向该键写入变更事件负载，
// 各进程的监听方收到事件后重读数据库并整体重建运行时装配态，保证配置热同步、无需重启。
// ChangeKey is the global etcd key of the LLM config change broadcast. An admin write path
// (model registration, slot binding, parameter override) writes the change event payload to
// this key after the DB commit; the watcher in each process re-reads the database and wholly
// rebuilds the runtime assembly state, keeping the config hot-synced without a restart.
var ChangeKey = etcd.WithPrefix("ai/") + "change"

// ChangeKind 是 LLM 配置变更的种类标识。
// ChangeKind is the kind identifier of an LLM config change.
type ChangeKind string

// 变更种类常量：模型注册表、档位绑定与参数覆盖三类写路径。
// Change kind constants: the three write paths of the model registry, the slot bindings and
// the parameter overrides.
const (
	// ChangeKindModel 模型注册表（llm_models）发生增删改
	// ChangeKindModel a create/update/delete on the model registry (llm_models)
	ChangeKindModel ChangeKind = "model"
	// ChangeKindBinding 档位绑定（llm_bindings）发生增删改
	// ChangeKindBinding a create/update/delete on the slot bindings (llm_bindings)
	ChangeKindBinding ChangeKind = "binding"
	// ChangeKindParam Agent 行为参数覆盖（agent_params）发生增删改
	// ChangeKindParam a create/update/delete on the agent parameter overrides (agent_params)
	ChangeKindParam ChangeKind = "param"
)

// ChangeEvent 是 LLM 配置变更事件的负载。
// 记录变更种类、受影响对象的本地标识（如模型名、档位、参数键）与发生时刻，
// 供监听方记录上下文；监听逻辑统一为整体重建，不依赖负载内容。
// ChangeEvent is the payload of an LLM config change event. It records the change kind, the
// local identity of the affected object (e.g. a model name, a slot, a parameter key) and the
// occurrence time for the watcher to log context; the watching logic is a wholesale rebuild
// and does not depend on the payload.
type ChangeEvent struct {
	// Kind 变更种类（model / binding / param）
	// Kind the change kind (model / binding / param)
	Kind ChangeKind `json:"kind"`
	// Name 受影响对象的本地标识，如模型名、档位标识或参数键
	// Name the local identity of the affected object, e.g. a model name, a slot or a
	// parameter key
	Name string `json:"name"`
	// At 变更发生时刻（未设置时由广播方填充为当前时间）
	// At the occurrence time of the change (filled with the current time by the broadcaster
	// when unset)
	At time.Time `json:"at"`
}

// BroadcastChange 广播 LLM 配置变更事件到 etcd 全局键。
// 管理端写路径在 DB 事务提交后调用，通知各进程的监听方重读数据库并重建运行时；
// 委托 etcd 通用广播层（JSON 序列化 + Put + 超时上限）；未绑定 etcd 客户端时返回
// etcd.ErrChangeUnavailable，由写路径告警降级而不影响已提交的业务变更。
// BroadcastChange broadcasts an LLM config change event to the global etcd key. An admin
// write path calls it after the DB transaction commits, telling the watcher in each process
// to re-read the database and rebuild the runtime; it delegates to the generic etcd
// broadcast layer (JSON serialization + Put + a timeout bound). With no etcd client bound it
// returns etcd.ErrChangeUnavailable, and the write path warns and degrades without affecting
// the committed change.
func BroadcastChange(ctx context.Context, client *etcd.EtcdClient, event *ChangeEvent) error {
	if client == nil {
		return etcd.ErrChangeUnavailable
	}
	if event == nil {
		return errors.New("ai change broadcast: nil event")
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	return etcd.BroadcastChange(ctx, client, ChangeKey, event)
}

// newWatcher 创建 LLM 配置变更监听器：订阅全局变更键，收到任一变更事件即整体重建运行时
// 装配态（Invalidate），重建失败由监听层记录告警并保留上一可用快照。
// newWatcher creates the LLM config change watcher: it subscribes to the global change key
// and wholly rebuilds the runtime assembly state (Invalidate) on any change event; a failed
// rebuild is warned by the watching layer, keeping the previous usable snapshot.
func newWatcher(rt *Runtime, client *etcd.EtcdClient, logger *logging.Logger) *etcd.Watcher {
	return etcd.NewWatcher(client, ChangeKey, etcd.ChangeHandlerFunc(func(ctx context.Context, _ []byte) error {
		return rt.Invalidate(ctx)
	}), logger)
}

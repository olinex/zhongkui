// Package change 承载 LLM 配置变更的跨进程广播与监听：管理端写路径在 DB 提交后广播变更事件，
// 各进程订阅同一 etcd 键并在收到事件后重读数据库、整体重建运行时装配态（热同步，无需重启）。
//
// 本包只负责「事件负载 + 广播 + 监听器构造」，不解读事件内容、不执行重建逻辑——重建由订阅方
// 注入的处理函数决定（internal/ai 的 Runtime.Invalidate）。
package change

import (
	// 标准库
	"context"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// ChangeKey 是 LLM 配置变更广播在 etcd 中的全局键。
// 管理端写路径（模型注册、档位绑定、参数覆盖）在 DB 提交后向该键写入变更事件负载，
// 各进程的监听方收到事件后重读数据库并整体重建运行时装配态，保证配置热同步、无需重启。
var ChangeKey = etcd.WithPrefix("ai/") + "change"

// ChangeKind 是 LLM 配置变更的种类标识。
type ChangeKind string

// 变更种类常量：模型注册表、档位绑定、参数覆盖、MCP 服务器注册表与工具注册表五类写路径。
const (
	// ChangeKindModel 模型注册表（llm_models）发生增删改
	ChangeKindModel ChangeKind = "model"
	// ChangeKindBinding 档位绑定（llm_bindings）发生增删改
	ChangeKindBinding ChangeKind = "binding"
	// ChangeKindParam Agent 行为参数覆盖（agent_params）发生增删改
	ChangeKindParam ChangeKind = "param"
	// ChangeKindMcp MCP 服务器注册表（mcp_servers）发生增删改
	ChangeKindMcp ChangeKind = "mcp"
	// ChangeKindTool 工具注册表（llm_tools）发生增删改
	ChangeKindTool ChangeKind = "tool"
)

// ChangeEvent 是 LLM 配置变更事件的负载。
// 记录变更种类、受影响对象的本地标识（如模型名、档位、参数键）与发生时刻，
// 供监听方记录上下文；监听逻辑统一为整体重建，不依赖负载内容。
type ChangeEvent struct {
	// Kind 变更种类（model / binding / param / mcp / tool）
	Kind ChangeKind `json:"kind"`
	// Name 受影响对象的本地标识，如模型名、档位标识或参数键
	Name string `json:"name"`
	// At 变更发生时刻（未设置时由广播方填充为当前时间）
	At time.Time `json:"at"`
}

// BroadcastChange 广播 LLM 配置变更事件到 etcd 全局键。
// 管理端写路径在 DB 事务提交后调用，通知各进程的监听方重读数据库并重建运行时；
// 委托 etcd 通用广播层（JSON 序列化 + Put + 超时上限）；未绑定 etcd 客户端时返回
// etcd.ErrChangeUnavailable，由写路径告警降级而不影响已提交的业务变更。
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

// NewWatcher 创建 LLM 配置变更监听器：订阅全局变更键，收到任一变更事件即调用 onEvent
// （订阅方据此重读数据库并整体重建装配态），失败由监听层记录告警且不中断后续事件。
// client 为 nil 时返回未连接的监听器，由调用方决定是否启动。
func NewWatcher(client *etcd.EtcdClient, onEvent func(ctx context.Context) error, logger *logging.Logger) *etcd.Watcher {
	return etcd.NewWatcher(client, ChangeKey, etcd.ChangeHandlerFunc(func(ctx context.Context, _ []byte) error {
		return onEvent(ctx)
	}), logger)
}

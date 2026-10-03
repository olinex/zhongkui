package datasource

import (
	// 标准库
	"context"
	"encoding/json"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
)

// datasourceChangeKey 数据源变更广播在 etcd 中的全局键。
// 写路径在 DB 提交并向本地池收敛注册后向该键写入变更事件负载，
// 各进程的 Watcher 监听该键并按 (Namespace, Name) 精准同步本地池，
// 保证多进程（web / worker）下数据源配置热同步、无需重启。
var datasourceChangeKey = etcd.WithPrefix("datasource/") + "change"

// DatasourceChangeEvent 数据源变更事件负载。
// 记录本次写路径影响的数据源身份（命名空间 + 裸名），供 Watcher 从 DB 重读
// 并收敛对应 vtable；解析失败时回退全量重载保证不遗漏。
type DatasourceChangeEvent struct {
	// Namespace 数据源所属命名空间
	Namespace string `json:"namespace"`
	// Name 数据源名称（命名空间内裸名 vtable）
	Name string `json:"name"`
}

// BroadcastChange 广播数据源变更事件。
// 每次数据源写路径（增删改/启停）在 DB 提交并向本地池收敛注册后调用，
// 通知各 worker 进程的 Watcher 同步对应数据源 vtable。
// 委托 etcd 通用广播层：JSON 序列化 + Put + 超时上限，失败返回错误但不影响
// 调用方；未绑定 etcd 客户端时返回 etcd.ErrChangeUnavailable。
// 注意：DB 提交先于 etcd 发布，保证收到通知的进程读到的必是新配置。
func BroadcastChange(ctx context.Context, client *etcd.EtcdClient, namespace, name string) error {
	return etcd.BroadcastChange(ctx, client, datasourceChangeKey, &DatasourceChangeEvent{Namespace: namespace, Name: name})
}

// parseDatasourceChangeEvent 解析数据源变更事件负载。
// 负载不是合法 JSON 或身份信息缺失时返回错误，调用方应回退全量重载。
func parseDatasourceChangeEvent(value []byte) (*DatasourceChangeEvent, error) {
	event := &DatasourceChangeEvent{}
	if err := json.Unmarshal(value, event); err != nil {
		return nil, errors.WithStack(err)
	}
	if event.Namespace == "" || event.Name == "" {
		return nil, errors.New("datasource change event misses identity info")
	}
	return event, nil
}

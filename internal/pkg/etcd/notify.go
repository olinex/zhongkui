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
	// stdlib
	"context"
	"encoding/json"
	"time"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

const (
	// publishTimeout 单次变更广播发布的超时上限。
	// etcd 端点不可达（如测试环境、etcd 故障）且未配置 DialTimeout 时，
	// Put 的 gRPC 重试会无限等待；加超时保证发布快速失败，
	// 由写路径告警降级而非阻塞响应。
	publishTimeout = 3 * time.Second
)

// ErrChangeUnavailable 变更广播不可用的错误。
// 未绑定 etcd 客户端（纯本地/测试场景）时广播无法发出，
// 写路径收到该错误仅告警降级，不影响已提交的业务变更。
var ErrChangeUnavailable = errors.New("etcd change broadcast unavailable: no etcd client")

// BroadcastChange 广播变更事件到 etcd 指定键。
// 业务写路径在 DB 事务提交后调用，通知所有进程的监听方按最新数据执行
// 同步/失效逻辑；发布带超时上限，失败返回错误但不影响调用方；
// 未绑定 etcd 客户端时返回 ErrChangeUnavailable。
// 注意：调用方应保证 DB 提交先于 etcd 发布，收到通知的进程读到的必是新数据。
func BroadcastChange(ctx context.Context, client *EtcdClient, key string, event any) error {
	if client == nil {
		return ErrChangeUnavailable
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if _, err := client.Put(ctx, key, string(payload)); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

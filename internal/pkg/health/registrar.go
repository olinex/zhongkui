package health

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etcd"
)

// ServiceInfo 写入 etcd 服务目录的单实例状态负载。
type ServiceInfo struct {
	// ServiceName 服务名称
	ServiceName string `json:"serviceName"`
	// InstanceID 实例唯一标识
	InstanceID string `json:"instanceId"`
	// Host 实例所在主机地址
	Host string `json:"host"`
	// Version 实例运行版本
	Version string `json:"version"`
	// PID 进程 ID
	PID int `json:"pid"`
	// StartedAt 实例启动时间
	StartedAt time.Time `json:"startedAt"`
	// Live 存活探针状态
	Live bool `json:"live"`
	// Ready 就绪探针状态
	Ready bool `json:"ready"`
	// Started 启动探针状态
	Started bool `json:"started"`
}

// registrar 经 etcd 租赁持续注册/续约服务运行状态；
// 键为 "{GlobalPrefix}/{KeyPrefix}/{serviceName}/{instanceID}"，值为 ServiceInfo JSON，
// 状态变化时更新，退出时吊销租赁使实例从目录消失。
type registrar struct {
	client *clientv3.Client
	server *Server
	cfg    RegistryConfig

	mu    sync.Mutex
	key   string
	lease clientv3.LeaseID
}

// newRegistrar 创建 etcd 服务状态注册器。
func newRegistrar(client *clientv3.Client, server *Server, cfg RegistryConfig) *registrar {
	return &registrar{client: client, server: server, cfg: cfg}
}

// keyOf 基于服务名与实例 ID 构造完整键。
func (r *registrar) keyOf(serviceName, instanceID string) string {
	return etcd.WithPrefix(r.cfg.KeyPrefix) + serviceName + "/" + instanceID
}

// run 持续注册并续约服务状态，直到 ctx 取消时吊销租赁。
func (r *registrar) run(ctx context.Context) error {
	if !r.cfg.Enabled || r.client == nil {
		<-ctx.Done()
		return nil
	}

	ttl := int64(r.cfg.TTL.Seconds())
	if ttl < 1 {
		ttl = 1
	}
	grant, err := r.client.Grant(ctx, ttl)
	if err != nil {
		return errors.WithStack(err)
	}
	r.lease = grant.ID

	if keepCh, err := r.client.KeepAlive(ctx, r.lease); err == nil {
		go func() {
			for range keepCh {
				// 持续续约；ctx 取消后通道关闭，循环随之退出。
			}
		}()
	}

	snap := r.server.Snapshot(ctx)
	r.key = r.keyOf(snap.ServiceName, snap.InstanceID)
	var last string
	if err := r.writeDeadline(ctx, &last); err != nil {
		return errors.WithStack(err)
	}

	interval := r.cfg.TTL / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			revoke := r.lease
			r.lease = 0
			r.mu.Unlock()
			if revoke != 0 {
				_, revokeErr := r.client.Revoke(context.Background(), revoke)
				return errors.WithStack(revokeErr)
			}
			return nil
		case <-ticker.C:
			if err := r.writeDeadline(ctx, &last); err != nil {
				return errors.WithStack(err)
			}
		}
	}
}

// writeDeadline 仅在状态发生变更时更新 etcd 键值。
func (r *registrar) writeDeadline(ctx context.Context, last *string) error {
	snap := r.server.Snapshot(ctx)
	info := ServiceInfo{
		ServiceName: snap.ServiceName,
		InstanceID:  snap.InstanceID,
		Host:        snap.Host,
		Version:     snap.Version,
		PID:         snap.PID,
		StartedAt:   snap.StartedAt,
		Live:        snap.Live,
		Ready:       snap.Ready,
		Started:     snap.Started,
	}
	b, err := json.Marshal(info)
	if err != nil {
		return errors.WithStack(err)
	}
	value := string(b)

	r.mu.Lock()
	defer r.mu.Unlock()
	if value == *last {
		return nil
	}
	r.key = r.keyOf(snap.ServiceName, snap.InstanceID)
	if _, err := r.client.Put(ctx, r.key, value, clientv3.WithLease(r.lease)); err != nil {
		return errors.WithStack(err)
	}
	*last = value
	return nil
}

// revoke 吊销租赁，使实例从服务目录消失。
func (r *registrar) revoke(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lease != 0 {
		_, _ = r.client.Revoke(ctx, r.lease)
		r.lease = 0
	}
}

// Package etcd 提供了 etcd 客户端的配置管理与连接创建功能。
// 该包封装了 etcd client/v3 客户端的配置结构，支持 TLS 安全连接，
// 并提供了配置描述和配置提取的便捷方法，方便从 viper 配置中加载 etcd 相关配置。
//
// 此外，该包还基于 etcd concurrency 包实现了分布式集群成员发现机制，
// 支持多实例场景下的成员注册、自动发现和基于一致性哈希的数据分片路由。
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/errgroupx"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/hash"
)

var (
	// ClusterEtcdPrefix 集群成员注册数据在 etcd 中的公共键前缀。
	ClusterEtcdPrefix = WithPrefix("cluster/")
)

// Cluster 基于 etcd 实现分布式集群成员发现与数据分片路由。
//
// 每个成员通过 etcd Session（带 TTL 租约）注册自身，并通过 Watch 机制
// 实时感知集群中其他成员的加入和离开。内部使用一致性哈希环（虚拟节点）
// 将数据键映射到成员，成员增删时仅迁移 O(1/N) 的数据。
//
// 使用模式（与 Cortex/Loki/Thanos 等项目的 ring 模式一致）：
//   - 多进程协作处理数据流时，按 key 哈希将数据分发给不同进程
//   - Session 租约自动过期机制保证进程崩溃后成员自动移除
//   - 成员变化时通过读写锁保护，不影响正在处理的数据
type Cluster struct {
	// mu 读写锁，保护一致性哈希环的并发访问。
	mu sync.RWMutex
	// client etcd 客户端实例，用于成员注册与监听。
	client *EtcdClient
	// name 集群名称（etcd 键识别名，隔离不同业务的集群）。
	name string
	// memberID 当前成员的唯一标识。
	memberID string
	// ring 一致性哈希环（虚拟节点），用于数据分片路由。
	ring *hash.ConsistentHash
}

// ClusterMember 表示集群成员的元信息。
// 注册时写入 etcd value，便于运维排查。
type ClusterMember struct {
	// MemberID 成员唯一标识
	MemberID string `json:"memberID"`
	// Hostname 主机名
	Hostname string `json:"hostname"`
	// RegisteredAt 注册时间
	RegisteredAt time.Time `json:"registeredAt"`
}

// NewCluster 创建集群管理器。
//
// 参数：
//   - client: etcd 客户端实例，必须已成功连接到 etcd 集群
//   - name: etcd 键识别名，用于隔离不同业务的集群成员。
//     推荐使用 WithPrefix() 生成，如 WithPrefix("workers/flow-ipv4/")
//   - memberID: 当前成员唯一标识，为空时自动生成（hostname + 随机后缀）
//
// 返回值：
//   - *Cluster: 初始化完成的集群管理器，尚未加入集群
func NewCluster(client *EtcdClient, name string) *Cluster {
	hostname := base.Hostname()
	rid, err := crypto.GenerateRandomString(8)
	if err != nil {
		panic(err)
	}
	memberID := fmt.Sprintf("%s-%s-%s", name, hostname, rid)
	return &Cluster{
		mu:       sync.RWMutex{},
		client:   client,
		name:     name,
		memberID: memberID,
		ring:     hash.NewConsistentHashWithDefaultVnodes(),
	}
}

// Name 返回集群名称。
func (c *Cluster) Name() string {
	return c.name
}

// MemberID 返回当前成员的唯一标识。
func (c *Cluster) MemberID() string {
	return c.memberID
}

func (c *Cluster) prefix() string {
	return fmt.Sprintf(`%s%s`, ClusterEtcdPrefix, c.name)
}

func (c *Cluster) key() string {
	return fmt.Sprintf(`%s/%s`, c.prefix(), c.memberID)
}

// Transaction 在集群会话中执行事务逻辑。
//
// 该方法是 Cluster 的核心方法，在事务期间完成以下步骤：
//  1. 持读锁创建 etcd Session（30s TTL），注册当前成员到 etcd
//  2. 释放读锁，持写锁加载全量成员列表并重建一致性哈希环
//  3. 后台启动成员变化监听（watchPeers），成员增删时自动重建哈希环
//  4. 执行传入的 tx 事务函数
//
// 锁策略说明：
//   - Session 创建和成员注册阶段仅持 RLock（读锁），确保不影响其他 goroutine
//     正在进行的 ShouldProcess/HashesOf 等读操作
//   - 注册完成后释放 RLock，再执行 rebuildRing（内部获取写锁），避免持有
//     读锁时再请求写锁导致死锁
//   - rebuildRing 重建哈希环期间写锁阻塞所有读操作，但耗时极短
func (c *Cluster) Transaction(ctx context.Context, tx func(ctx context.Context) error) error {
	c.mu.RLock()
	session, err := concurrency.NewSession(
		c.client,
		concurrency.WithContext(ctx),
		concurrency.WithTTL(30),
	)
	if err != nil {
		c.mu.RUnlock()
		return errors.WithStack(err)
	}

	// 注册本成员
	memberInfo := ClusterMember{
		MemberID:     c.memberID,
		Hostname:     base.Hostname(),
		RegisteredAt: time.Now(),
	}
	metaBytes, err := json.Marshal(memberInfo)
	if err != nil {
		c.mu.RUnlock()
		return errors.WithStack(err)
	}
	if _, err := c.client.Put(ctx, c.key(), string(metaBytes), clientv3.WithLease(session.Lease())); err != nil {
		c.mu.RUnlock()
		return errors.WithStack(err)
	}
	c.mu.RUnlock()

	// 首次加载成员列表
	if err := c.rebuildRing(ctx); err != nil {
		return errors.WithStack(err)
	}

	wg, innerCtx := errgroupx.WithContext(ctx)
	// 后台监听成员变化
	wg.GoNamed("watch-peers", func() error {
		c.watchPeers(ctx)
		return nil
	})
	// 执行事务
	wg.GoNamed("tx", func() error {
		return tx(innerCtx)
	})
	if err := wg.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// ShouldProcess 判断当前成员是否应处理指定 key 的数据。
// 使用一致性哈希环查找 key 对应的成员，返回该成员是否为当前成员。
//
// 参数：
//   - key: 数据分片键（如 streamID）
//
// 返回值：
//   - bool: true 表示当前成员负责处理该 key 的数据
func (c *Cluster) ShouldProcess(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ring.Get(key) == c.memberID
}

// MemberCount 返回当前活跃成员数量。
func (c *Cluster) MemberCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ring.MemberCount()
}

// HashesOf 返回指定成员在一致性哈希环上的所有虚拟节点哈希位置。
// 委托给内部 ConsistentHash.HashesOf 实现。
func (c *Cluster) HashesOf(member string) []uint32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ring.HashesOf(member)
}

// VNodeIndicesOf 返回指定成员在数据分片中所属的虚拟节点索引列表。
// 用于 ClickHouse 侧按 cityHash64(key) % VNodeCount 过滤数据。
func (c *Cluster) VNodeIndicesOf(member string) []int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ring.VNodeIndicesOf(member)
}

// rebuildRing 重新加载成员列表并构建一致性哈希环。
func (c *Cluster) rebuildRing(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	resp, err := c.client.Get(ctx, c.prefix(), clientv3.WithPrefix())
	if err != nil {
		return errors.WithStack(err)
	}

	// 精确匹配当前集群名的 key 前缀，避免误匹配到其他前缀（如 zki/clusters/）
	prefix := c.prefix()

	members := make([]string, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		if kv == nil {
			continue
		}
		// 过滤不属于当前集群的 key（如 zki/clusters/ 下的水位游标）
		if !strings.HasPrefix(string(kv.Key), prefix) {
			continue
		}
		var info ClusterMember
		if err := json.Unmarshal(kv.Value, &info); err != nil {
			continue
		}
		members = append(members, info.MemberID)
	}
	sort.Strings(members)
	c.ring.Update(members)
	return nil
}

// watchPeers 持续监听 etcd prefix 下的成员变化，变化时重建一致性哈希环。
func (c *Cluster) watchPeers(ctx context.Context) {
	watchChan := c.client.Watch(ctx, c.prefix(), clientv3.WithPrefix())
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-watchChan:
			if !ok {
				return
			}
			// 任何成员变化都触发重建
			if err := c.rebuildRing(ctx); err != nil {
				// 关机时 ctx 已取消，重建失败属于正常退出路径，不能 panic。
				if ctx.Err() != nil {
					return
				}
				panic(err)
			}
		}
	}
}
